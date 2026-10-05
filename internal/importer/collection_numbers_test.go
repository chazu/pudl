package importer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyItemHash is the content hash the float64 collection path computed:
// json.Unmarshal into interface{}, then json.Marshal.
func legacyItemHash(t *testing.T, record string) string {
	t.Helper()
	var value interface{}
	require.NoError(t, json.Unmarshal([]byte(record), &value))
	canonical, err := json.Marshal(value)
	require.NoError(t, err)
	return idgen.ComputeContentID(canonical)
}

func TestImportCollection_PreservesIntegersBeyondFloat64(t *testing.T) {
	content := `{"id":9007199254740993,"name":"a"}` + "\n" +
		`{"id":9007199254740995,"name":"b"}` + "\n"
	imp, source, _ := collectionFixture(t, "big.ndjson", content)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)

	want := []string{"9007199254740993", "9007199254740995"}
	for i, item := range items {
		stored, err := os.ReadFile(item.StoredPath)
		require.NoError(t, err)
		assert.Contains(t, string(stored), `"id": `+want[i], "stored item keeps every digit")
		assert.False(t, strings.HasSuffix(string(stored), "\n"), "stored item has no trailing newline")
	}
	assert.NotEqual(t, items[0].ID, items[1].ID, "distinct large integers hash distinctly")
}

func TestImportCollection_StoresRecordBytesAsWritten(t *testing.T) {
	record := `{"z":1.50,"a":"<b>","n":1e3}`
	imp, source, _ := collectionFixture(t, "spelled.ndjson", record+"\n")

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

	stored, err := os.ReadFile(items[0].StoredPath)
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"z\": 1.50,\n  \"a\": \"<b>\",\n  \"n\": 1e3\n}", string(stored),
		"the record's own key order, number spellings and string escapes are retained")
}

func TestImportCollection_ContentHashMatchesLegacyFloatPath(t *testing.T) {
	// Values that survive a float64 round trip must keep the hash the earlier
	// path gave them, so a re-import deduplicates against items stored before.
	records := []string{
		`{"n":1.0,"name":"one"}`,
		`{"n":1e3,"name":"thousand"}`,
		`{"nested":[1e-7,{"x":-0.125}],"n":-0,"s":"<&>"}`,
		`{"n":9007199254740992}`,
	}
	imp, source, _ := collectionFixture(t, "legacy.ndjson", strings.Join(records, "\n")+"\n")

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	require.Len(t, items, len(records))

	for i, item := range items {
		assert.Equal(t, legacyItemHash(t, records[i]), item.ID, "record %d", i)
	}
}

package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectionFixture stages a workspace, an importer, and a source file.
func collectionFixture(t *testing.T, name, content string) (*EnhancedImporter, string, string) {
	t.Helper()
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	source := setup.WriteFile(name, content)

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = imp.Close() })
	return imp, source, workspace.DataDir
}

func ndjsonRecords(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `{"name":"resource-%d","index":%d}`+"\n", i, i)
	}
	return b.String()
}

func TestImportCollection_NDJSONStreamsEveryRecord(t *testing.T) {
	imp, source, _ := collectionFixture(t, "records.ndjson", ndjsonRecords(25))

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 25, result.RecordCount)

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	require.Len(t, items, 25, "every streamed record became an item")
	for i, item := range items {
		require.NotNil(t, item.ItemIndex)
		assert.Equal(t, i, *item.ItemIndex, "membership order follows stream order")
		assert.FileExists(t, item.StoredPath)
	}
}

func TestImportCollection_JSONArrayStreams(t *testing.T) {
	// The case the architecture report calls out. It used to go through the
	// chunking parser, which materialized every element before writing a row.
	var records []string
	for i := 0; i < 10; i++ {
		records = append(records, fmt.Sprintf(`{"name":"resource-%d","index":%d}`, i, i))
	}
	imp, source, _ := collectionFixture(t, "records.json", "[\n"+strings.Join(records, ",\n")+"\n]")

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 10, result.RecordCount)

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	assert.Len(t, items, 10)
}

func TestImportCollection_ContentHashIsTheRawFileHash(t *testing.T) {
	content := ndjsonRecords(5)
	imp, source, _ := collectionFixture(t, "records.ndjson", content)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)

	want := sha256.Sum256([]byte(content))
	assert.Equal(t, hex.EncodeToString(want[:]), result.ContentHash,
		"identity is SHA256 of the raw source bytes, unchanged by how the copy is made")
	assert.Equal(t, result.ContentHash, result.ID)
}

func TestImportCollection_RepeatedRecordDeduplicates(t *testing.T) {
	// A record repeated within one file is one entry with two memberships — and
	// the dedup read goes through the transaction, so it sees the copy this same
	// import just wrote.
	duplicate := `{"name":"same","index":0}` + "\n"
	imp, source, _ := collectionFixture(t, "records.ndjson", duplicate+duplicate+`{"name":"other","index":1}`+"\n")

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	assert.Equal(t, 3, result.RecordCount, "three records were streamed")

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	assert.Len(t, items, 2, "but the repeated record is one entry")
}

func TestImportCollection_FailureLeavesNoRowsAndNoFiles(t *testing.T) {
	// All-or-nothing, now enforced by the transaction rather than unwound by hand
	// through a reconstructed-path cleanup.
	imp, source, dataDir := collectionFixture(t, "records.ndjson",
		ndjsonRecords(3)+"this line is not json\n")

	_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.Error(t, err)

	entries, err := imp.catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	assert.Empty(t, entries.Entries, "no collection row and no item rows survive the failure")

	// The item artifacts the failed stream wrote are gone too.
	rawRoot := filepath.Join(dataDir, "raw")
	var itemFiles []string
	_ = filepath.Walk(rawRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.Contains(filepath.Base(path), "_item_") {
			itemFiles = append(itemFiles, path)
		}
		return nil
	})
	assert.Empty(t, itemFiles, "and so are the item files it staged")
}

func TestImportCollection_DuplicateFileLeavesNoStagedCopy(t *testing.T) {
	content := ndjsonRecords(4)
	imp, source, dataDir := collectionFixture(t, "records.ndjson", content)

	first, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	require.False(t, first.Skipped)

	second, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	assert.True(t, second.Skipped, "the same bytes are already in the catalog")

	// Staging happens before the dedup check, so the discarded copy must be gone.
	var staging []string
	_ = filepath.Walk(filepath.Join(dataDir, "raw"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".staging-") {
			staging = append(staging, path)
		}
		return nil
	})
	assert.Empty(t, staging, "a duplicate import leaves no staged copy behind")
}

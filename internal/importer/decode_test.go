package importer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cases reproduce defects of the content-defined-chunking parser the
// decoders replaced: it dropped any chunk whose bytes it had seen before, so
// repeated byte runs vanished from parsed values and documents with repeated
// elements failed to parse.

func TestDecodeJSON_RepeatedValuePreservedExactly(t *testing.T) {
	blob := strings.Repeat("z", 300000)
	src, err := json.Marshal(map[string]interface{}{"kind": "Thing", "blob": blob, "tail": "end"})
	require.NoError(t, err)

	value, count, err := decodeDocument(strings.NewReader(string(src)), "json")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	object := value.(map[string]interface{})
	assert.Equal(t, blob, object["blob"], "every byte of a repeated run survives")
	assert.Equal(t, "end", object["tail"])
}

func TestDecodeJSON_IdenticalLargeItemsParse(t *testing.T) {
	labels := map[string]string{}
	for i := 0; i < 1200; i++ {
		labels[fmt.Sprintf("team.example.com/label-%04d", i)] = "shared-value-for-all-pods"
	}
	items := make([]interface{}, 10)
	for i := range items {
		items[i] = map[string]interface{}{"kind": "Pod", "metadata": map[string]interface{}{"name": "same", "labels": labels}}
	}
	src, err := json.Marshal(map[string]interface{}{"apiVersion": "v1", "kind": "List", "items": items})
	require.NoError(t, err)
	require.Greater(t, len(src)/10, 60000, "each item is large enough to span several old chunks")

	value, _, err := decodeDocument(strings.NewReader(string(src)), "json")
	require.NoError(t, err)
	parsed := value.(map[string]interface{})["items"].([]interface{})
	assert.Len(t, parsed, 10)
}

func TestDecodeJSON_LargeIntegersKeepPrecision(t *testing.T) {
	value, _, err := decodeDocument(strings.NewReader(`{"id": 9007199254740993}`), "json")
	require.NoError(t, err)
	assert.Equal(t, json.Number("9007199254740993"), value.(map[string]interface{})["id"])
}

func TestDecodeJSON_RejectsTrailingContent(t *testing.T) {
	_, _, err := decodeDocument(strings.NewReader(`{"a":1} {"b":2}`), "json")
	assert.Error(t, err)
}

func TestDecodeCSV_EveryRowVerbatim(t *testing.T) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	require.NoError(t, w.Write([]string{"id", "name", "zip", "note"}))
	for i := 0; i < 300; i++ {
		note := fmt.Sprintf("n%d", i)
		if i%5 == 0 {
			note = ""
		}
		require.NoError(t, w.Write([]string{fmt.Sprint(i), fmt.Sprintf("name-%d", i), fmt.Sprintf("%05d", i), note}))
	}
	w.Flush()

	value, count, err := decodeDocument(strings.NewReader(b.String()), "csv")
	require.NoError(t, err)
	assert.Equal(t, 300, count)

	rows := value.([]map[string]string)
	require.Len(t, rows, 300)
	for i, row := range rows {
		assert.Equal(t, fmt.Sprint(i), row["id"])
		assert.Equal(t, fmt.Sprintf("name-%d", i), row["name"])
		assert.Equal(t, fmt.Sprintf("%05d", i), row["zip"], "leading zeros are kept")
	}
	assert.Equal(t, "00001", rows[1]["zip"])
	assert.Equal(t, "", rows[0]["note"], "an empty cell stays empty")
}

func TestDecodeYAML_MultipleDocuments(t *testing.T) {
	value, count, err := decodeDocument(strings.NewReader("a: 1\n---\nb: 2\n"), "yaml")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Len(t, value.([]interface{}), 2)
}

func TestDecodeYAML_LargeDocumentIsOneRecord(t *testing.T) {
	var b strings.Builder
	b.WriteString("kind: Config\nentries:\n")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "  - name: entry-%d\n    value: %s\n", i, strings.Repeat("v", 20))
	}
	value, count, err := decodeDocument(strings.NewReader(b.String()), "yaml")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, value.(map[string]interface{})["entries"], 2000)
}

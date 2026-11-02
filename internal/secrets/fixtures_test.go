package secrets

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoFindingsInCloudFixtures guards the false-positive rate: realistic
// gcloud, aws and kubectl output (resource IDs, self-links, etags,
// fingerprints, digests, UUIDs, base64 CA certificates) and the repository's
// own fixtures must produce no findings.
func TestNoFindingsInCloudFixtures(t *testing.T) {
	globs := []string{
		"testdata/cloud/*.ndjson",
		"../../test/data/*.json",
		"../../test/integration/testdata/type_detection/*.json",
		"../../examples/*/*.json",
	}
	records := 0
	for _, glob := range globs {
		files, err := filepath.Glob(glob)
		require.NoError(t, err)
		require.NotEmpty(t, files, glob)
		for _, file := range files {
			for i, record := range decodeRecords(t, file) {
				records++
				assert.Empty(t, Scan(record, Options{}), "%s record %d", file, i)
			}
		}
	}
	assert.Greater(t, records, 70)
}

// decodeRecords reads a JSON document (an array yields its elements) or NDJSON.
func decodeRecords(t *testing.T, file string) []any {
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	if strings.HasSuffix(file, ".ndjson") {
		var out []any
		sc := bufio.NewScanner(bytes.NewReader(raw))
		sc.Buffer(make([]byte, 1<<20), 1<<22)
		for sc.Scan() {
			var v any
			require.NoError(t, json.Unmarshal(sc.Bytes(), &v), file)
			out = append(out, v)
		}
		return out
	}
	var v any
	require.NoError(t, json.Unmarshal(raw, &v), file)
	if arr, ok := v.([]any); ok {
		return arr
	}
	return []any{v}
}

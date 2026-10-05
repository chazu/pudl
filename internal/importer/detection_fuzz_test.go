package importer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// FuzzDetectFormat runs content-based detection (a file with no extension).
//
// Guaranteed properties:
//   - detection never panics and never errors on a readable file;
//   - a single valid JSON object or array that fits in the 4KB detection window
//     is detected as json or ndjson;
//   - whenever detection says ndjson, the content imports as NDJSON: every
//     non-blank line is valid JSON (checked only when the whole file is inside
//     the detection window, since detection samples the first 4KB).
func FuzzDetectFormat(f *testing.F) {
	for _, seed := range []string{
		`{"a":1}`,
		"{\"a\":1}\n{\"b\":2}\n",
		"[\n{\"id\":1}\n,\n{\"id\":2}\n]\n",
		"{\"a\":\n{\"b\":1}\n,\"c\":\n{\"d\":2}\n}\n",
		"a: 1\nb: 2\n",
		"id,name\n1,x\n",
		"plain text",
		"",
		"  [1, 2, 3]  ",
	} {
		f.Add([]byte(seed))
	}
	importer := &EnhancedImporter{}
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "data")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		format, err := importer.detectFormat(path)
		if err != nil {
			t.Fatalf("detectFormat error on %q: %v", data, err)
		}
		if len(data) > 4096 {
			return
		}
		trimmed := bytes.TrimSpace(data)
		isContainer := len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
		if isContainer && json.Valid(data) && format != "json" && format != "ndjson" {
			t.Fatalf("valid JSON %q detected as %q", data, format)
		}
		if format == "ndjson" {
			if _, err := streamNDJSON(bytes.NewReader(data), func(int, json.RawMessage) error { return nil }); err != nil {
				t.Fatalf("detected ndjson but %q does not import as NDJSON: %v", data, err)
			}
		}
	})
}

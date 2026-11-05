package fileset

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// manifestPath is where an install records the digests of the files it wrote.
func manifestPath(root string, kind Kind, name string) string {
	return filepath.Join(root, kind.RecordDir, name+".json")
}

// readManifest returns path -> sha256 of the files the last install wrote, or
// an empty map when the file set was never installed with a record.
func readManifest(root string, kind Kind, name string) (map[string]string, error) {
	data, err := os.ReadFile(manifestPath(root, kind, name))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]string{}, nil // unreadable: treat files as unrecorded (conflicts, never silent overwrites)
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m.Files, nil
}

func writeManifest(root string, kind Kind, name string, files map[string][]byte) error {
	m := map[string]any{kind.Noun: name}
	digests := map[string]string{}
	for path, data := range files {
		digests[path] = digest(data)
	}
	m["files"] = digests
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(manifestPath(root, kind, name), append(data, '\n'))
}

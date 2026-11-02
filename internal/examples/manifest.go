package examples

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// manifestPath is where an install records the digests of the files it wrote.
func manifestPath(root, name string) string {
	return filepath.Join(root, ".examples", name+".json")
}

// readManifest returns path -> sha256 of the files the last install wrote, or
// an empty map when the example was never installed by a pudl that records one.
func readManifest(root, name string) (map[string]string, error) {
	data, err := os.ReadFile(manifestPath(root, name))
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

func writeManifest(root, name string, files map[string][]byte) error {
	m := struct {
		Example string            `json:"example"`
		Files   map[string]string `json:"files"`
	}{Example: name, Files: map[string]string{}}
	for path, data := range files {
		m.Files[path] = digest(data)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(manifestPath(root, name), append(data, '\n'))
}

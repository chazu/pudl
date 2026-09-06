// Package examples installs bundled examples into initialized PUDL workspaces.
package examples

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gitinventory "github.com/chazu/pudl/examples/git-inventory"
)

// Install supplies the model, fixture inventories, and optional live observer.
// Existing identical files are left alone; any conflict fails before writing.
func Install(root, name string) ([]string, error) {
	if name != "git-inventory" {
		return nil, fmt.Errorf("unknown example %q (available: git-inventory)", name)
	}
	files, err := gitInventoryFiles(root)
	if err != nil {
		return nil, err
	}
	var paths []string
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var pending []string
	for _, path := range paths {
		if err := checkParents(root, filepath.Dir(path)); err != nil {
			return nil, err
		}
		info, err := os.Lstat(filepath.Join(root, path))
		if os.IsNotExist(err) {
			pending = append(pending, path)
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("example destination is not a regular file: %s", path)
		}
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(data, files[path]) {
			return nil, fmt.Errorf("example file already exists with different content: %s; preserving your file", path)
		}
	}
	for _, path := range pending {
		absolute := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, err
		}
		_, writeErr := f.Write(files[path])
		closeErr := f.Close()
		if writeErr != nil {
			return nil, writeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	return paths, nil
}

func gitInventoryFiles(root string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	model, err := gitinventory.Files.ReadFile("model.cue")
	if err != nil {
		return nil, err
	}
	files["schema/models/git_inventory.cue"] = model
	const populator = "populators/git-inventory/"
	for _, name := range []string{"observe.py", "baseline.json", "changed.json"} {
		data, err := gitinventory.Files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		files[populator+name] = data
		if strings.HasSuffix(name, ".json") {
			observation := []map[string]any{{
				"target":  "//git-inventory/demo",
				"current": map[string]any{"records": json.RawMessage(data)},
			}}
			encoded, err := json.MarshalIndent(observation, "", "  ")
			if err != nil {
				return nil, err
			}
			files[populator+strings.TrimSuffix(name, ".json")+"-observe.json"] = append(encoded, '\n')
		}
	}
	files[populator+"current.json"] = files[populator+"baseline.json"]
	cachePath := filepath.Join(root, "data", "mu", "cache")
	files["data/mu/mu.cue"] = []byte(fmt.Sprintf("package mu\ncache: backends: [{type: \"disk\", path: %q}]\n", cachePath))
	return files, nil
}

// Refuse symlinked destinations so example installation stays in its workspace.
func checkParents(root, relative string) error {
	for relative != "." {
		info, err := os.Lstat(filepath.Join(root, relative))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !info.IsDir() {
			return fmt.Errorf("example destination is not a directory: %s", relative)
		}
		relative = filepath.Dir(relative)
	}
	return nil
}

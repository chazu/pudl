// Package examples installs bundled examples into initialized PUDL workspaces.
package examples

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	gcppack "github.com/chazu/pudl/examples/gcp"
	gcpnetwork "github.com/chazu/pudl/examples/gcp-network-hygiene"
	gitinventory "github.com/chazu/pudl/examples/git-inventory"
)

// Install supplies the model, fixture inventories, and optional live observer.
// Existing identical files are left alone; any conflict fails before writing.
func Install(root, name string) ([]string, error) {
	var files map[string][]byte
	var err error
	switch name {
	case "git-inventory":
		files, err = gitInventoryFiles()
	case "gcp-network-hygiene":
		files, err = gcpNetworkFiles()
	case "gcp":
		files, err = treeFiles(gcppack.Files)
	default:
		return nil, fmt.Errorf("unknown example %q (available: git-inventory, gcp-network-hygiene, gcp)", name)
	}
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

func gitInventoryFiles() (map[string][]byte, error) {
	files := make(map[string][]byte)
	model, err := gitinventory.Files.ReadFile("model.cue")
	if err != nil {
		return nil, err
	}
	files["schema/models/git_inventory.cue"] = model
	const populator = "populators/git-inventory/"
	for _, name := range []string{"baseline.json", "changed.json"} {
		data, err := gitinventory.Files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		files[populator+name] = data
	}
	files[populator+"current.json"] = files[populator+"baseline.json"]
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

// treeFiles installs every file of a bundle at its path in the bundle.
func treeFiles(bundle fs.FS) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(bundle, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(bundle, path)
		files[path] = data
		return err
	})
	return files, err
}

func gcpNetworkFiles() (map[string][]byte, error) {
	files := map[string][]byte{}
	for source, destination := range map[string]string{"schema.cue": "schema/pudl/gcphygiene/firewall.cue", "model.cue": "schema/models/gcp_network_hygiene.cue", "baseline.json": "populators/gcp-network-hygiene/baseline.json", "fixed.json": "populators/gcp-network-hygiene/fixed.json"} {
		data, err := gcpnetwork.Files.ReadFile(source)
		if err != nil {
			return nil, err
		}
		files[destination] = data
	}
	files["populators/gcp-network-hygiene/current.json"] = files["populators/gcp-network-hygiene/baseline.json"]
	return files, nil
}

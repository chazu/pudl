package examples

import (
	"fmt"
	"io/fs"

	awspack "github.com/chazu/pudl/examples/aws"
	gcppack "github.com/chazu/pudl/examples/gcp"
	gcpnetwork "github.com/chazu/pudl/examples/gcp-network-hygiene"
	gitinventory "github.com/chazu/pudl/examples/git-inventory"
)

// packFiles returns an example's files by their path in the workspace.
func packFiles(name string) (map[string][]byte, error) {
	switch name {
	case "git-inventory":
		return gitInventoryFiles()
	case "gcp-network-hygiene":
		return gcpNetworkFiles()
	case "gcp":
		return treeFiles(gcppack.Files)
	case "aws":
		return treeFiles(awspack.Files)
	}
	return nil, fmt.Errorf("unknown example %q (available: git-inventory, gcp-network-hygiene, gcp, aws)", name)
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

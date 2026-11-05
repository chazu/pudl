// Package examples installs the bundled tutorials (pudl example install) into
// initialized PUDL workspaces. Installs are recorded under
// <workspace>/.examples; see package fileset for upgrades and conflicts.
package examples

import (
	"fmt"

	gcpnetwork "github.com/chazu/pudl/examples/gcp-network-hygiene"
	gitinventory "github.com/chazu/pudl/examples/git-inventory"
	"github.com/chazu/pudl/internal/fileset"
)

// Kind is the tutorial family of file sets.
var Kind = fileset.Kind{Noun: "example", RecordDir: ".examples"}

// Names lists the installable examples.
func Names() []string {
	return []string{"git-inventory", "gcp-network-hygiene"}
}

// Install writes example name into the workspace at root.
func Install(root, name string, opts fileset.Options) (*fileset.Result, error) {
	files, err := Files(name)
	if err != nil {
		return nil, err
	}
	return fileset.Install(root, Kind, name, files, opts)
}

// Files returns an example's files by their path in the workspace.
func Files(name string) (map[string][]byte, error) {
	switch name {
	case "git-inventory":
		return gitInventoryFiles()
	case "gcp-network-hygiene":
		return gcpNetworkFiles()
	}
	return nil, fmt.Errorf("unknown example %q (available: git-inventory, gcp-network-hygiene)", name)
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

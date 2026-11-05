// Package packs installs the bundled content packs (pudl pack install) into
// initialized PUDL workspaces. Installs are recorded under <workspace>/.packs;
// see package fileset for upgrades and conflicts.
package packs

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/chazu/pudl/internal/fileset"
	awspack "github.com/chazu/pudl/packs/aws"
	gcppack "github.com/chazu/pudl/packs/gcp"
)

// Kind is the content-pack family of file sets.
var Kind = fileset.Kind{Noun: "pack", RecordDir: ".packs"}

// Pack is one installable content pack.
type Pack struct {
	Name    string
	Summary string
	files   fs.FS
}

// all lists the packs in display order.
var all = []Pack{
	{Name: "gcp", Summary: "Google Cloud inventory schemas, policies, gcloud and fixture collectors; network, data and GKE checks", files: gcppack.Files},
	{Name: "aws", Summary: "AWS inventory schemas, policies, aws CLI and fixture collectors; network, data and identity checks", files: awspack.Files},
}

// List returns every pack in display order.
func List() []Pack {
	return append([]Pack(nil), all...)
}

// Names lists the installable packs.
func Names() []string {
	names := make([]string, len(all))
	for i, p := range all {
		names[i] = p.Name
	}
	return names
}

// Files returns a pack's files by their path in the workspace.
func Files(name string) (map[string][]byte, error) {
	for _, p := range all {
		if p.Name == name {
			return fileset.TreeFiles(p.files)
		}
	}
	return nil, fmt.Errorf("unknown pack %q (available: %s)", name, strings.Join(Names(), ", "))
}

// Install writes pack name into the workspace at root.
func Install(root, name string, opts fileset.Options) (*fileset.Result, error) {
	files, err := Files(name)
	if err != nil {
		return nil, err
	}
	return fileset.Install(root, Kind, name, files, opts)
}

// State reports how the workspace's install record of pack name compares
// with this pudl's version of it.
func State(root, name string) (fileset.State, error) {
	files, err := Files(name)
	if err != nil {
		return "", err
	}
	return fileset.RecordedState(root, Kind, name, files)
}

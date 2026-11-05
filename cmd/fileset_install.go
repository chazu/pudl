package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/fileset"
)

// filesetInstallRoot is the workspace a file set (example or pack) is installed
// into: the repository workspace, or ~/.pudl only when --global names it
// explicitly, so running outside a repository never writes to ~/.pudl by
// accident.
func filesetInstallRoot(cmd *cobra.Command, noun string) (string, error) {
	global, _ := cmd.Flags().GetBool("global")
	if !global {
		if wsPolicy == nil || !wsPolicy.InWorkspace() {
			return "", fmt.Errorf("%s installation needs a workspace; run pudl init here, or pass --global to install into ~/.pudl", noun)
		}
		return wsPolicy.Workspace.PudlDir, nil
	}
	root := wsPolicy.GlobalDir
	if _, err := os.Stat(config.ConfigPath(root)); err != nil {
		return "", fmt.Errorf("the global workspace %s is not initialized; run pudl init --global first", root)
	}
	return root, nil
}

// filesetInstaller installs one named file set into a workspace root
// (examples.Install, packs.Install).
type filesetInstaller func(root, name string, opts fileset.Options) (*fileset.Result, error)

// installFileset installs file set name of kind into the command's workspace and
// reports the result. It reports whether the result was written as JSON;
// callers then print nothing else.
func installFileset(cmd *cobra.Command, kind fileset.Kind, name string, install filesetInstaller) (bool, error) {
	root, err := filesetInstallRoot(cmd, kind.Noun)
	if err != nil {
		return false, err
	}
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	res, err := install(root, name, fileset.Options{Overwrite: overwrite})
	if err != nil {
		return false, err
	}
	syncProjectionsAfterInstall(cmd.Context(), root, res)
	if jsonOutput {
		return true, GetOutputWriter().WriteJSON(map[string]any{
			kind.Noun: name, "root": root, "files": res.Files, "added": res.Added,
			"upgraded": res.Upgraded, "replaced": res.Replaced, "unchanged": res.Unchanged, "backups": res.Backups,
		})
	}
	fmt.Fprintf(outw(), "Installed %s in %s: %d added, %d upgraded, %d replaced, %d unchanged\n",
		name, root, len(res.Added), len(res.Upgraded), len(res.Replaced), len(res.Unchanged))
	for _, path := range res.Replaced {
		fmt.Fprintf(outw(), "  replaced %s (yours kept as %s)\n", path, res.Backups[path])
	}
	return false, nil
}

// syncProjectionsAfterInstall reprojects stored records' facts when an install
// changed files: an upgrade can change a type's _pudl.facts specs, and records
// otherwise kept the facts the old specs projected until pudl doctor --fix
// --only projections. A workspace without a catalog has no records; its
// catalog is not created here.
func syncProjectionsAfterInstall(ctx context.Context, root string, res *fileset.Result) {
	if len(res.Added)+len(res.Upgraded)+len(res.Replaced) == 0 {
		return
	}
	if _, err := os.Stat(filepath.Join(root, "data", "sqlite", "catalog.db")); err != nil {
		return
	}
	syncProjectionsAfterWrite(ctx)
}

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/examples"
)

var exampleCmd = &cobra.Command{
	Use:   "example",
	Short: "Install bundled examples into this workspace",
}

var exampleInstallCmd = &cobra.Command{
	Use:       "install <name>",
	Short:     "Install a runnable example and its fixture data",
	ValidArgs: []string{"git-inventory", "gcp-network-hygiene", "gcp", "aws"},
	Args:      cobra.ExactArgs(1),
	Long: `Install a bundled example in the current initialized PUDL workspace,
or with --global in the personal ~/.pudl workspace (after pudl init --global).
No source checkout or download is needed.

Installing again upgrades the example: files you have not edited since pudl
installed them are replaced with this version's, and new files are added.
Files you edited, and files from an install by an older pudl (which kept no
record of what it wrote), are conflicts: nothing is written and each is
listed. --overwrite replaces them, keeping your versions as <file>.bak.

The git-inventory example includes a model expecting default branch main and
baseline and changed inventories. The model observes the current inventory
with cat, so the tutorial needs nothing beyond PUDL.

The gcp-network-hygiene example includes a firewall type and policy, a
fixture observer, a check over policy results, a corrected fixture, and a
separate gcloud-based live model.

The gcp content pack includes draft schemas for the core GCP inventory,
best-practice policies, gcloud and fixture collectors, and three check
families: network hygiene, data protection and GKE posture.

The aws content pack does the same for AWS, with aws CLI and fixture
collectors and three check families: network exposure, data protection and
identity posture.

Example:
    pudl init
    pudl example install git-inventory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		global, _ := cmd.Flags().GetBool("global")
		root, err := exampleInstallRoot(global)
		if err != nil {
			return err
		}
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		res, err := examples.Install(root, args[0], examples.Options{Overwrite: overwrite})
		if err != nil {
			return err
		}
		if jsonOutput {
			return GetOutputWriter().WriteJSON(map[string]any{
				"example": args[0], "root": root, "files": res.Files, "added": res.Added,
				"upgraded": res.Upgraded, "replaced": res.Replaced, "unchanged": res.Unchanged, "backups": res.Backups,
			})
		}
		fmt.Fprintf(outw(), "Installed %s in %s: %d added, %d upgraded, %d replaced, %d unchanged\n",
			args[0], root, len(res.Added), len(res.Upgraded), len(res.Replaced), len(res.Unchanged))
		for _, path := range res.Replaced {
			fmt.Fprintf(outw(), "  replaced %s (yours kept as %s)\n", path, res.Backups[path])
		}
		if args[0] == "aws" {
			fmt.Fprintln(outw(), "Next: pudl check aws-fixture-vpcs aws-fixture-subnets aws-fixture-security-groups aws-fixture-instances aws-fixture-network-exposure-checks (fixture; no cloud access)")
			return nil
		}
		if args[0] == "gcp" {
			fmt.Fprintln(outw(), "Next: pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers gcp-fixture-network-hygiene-checks (fixture; no cloud access)")
			return nil
		}
		if args[0] == "gcp-network-hygiene" {
			fmt.Fprintln(outw(), "Next: pudl check gcp-network-hygiene --detailed-exitcode (fixture; no cloud access)")
			return nil
		}
		fmt.Fprintf(outw(), "Model: git-inventory\nInventories: .pudl/populators/git-inventory/{baseline,changed}.json (the model observes current.json)\n")
		fmt.Fprintln(outw(), "Next: pudl model show git-inventory")
		return nil
	},
}

// exampleInstallRoot is the workspace an example is installed into: the
// repository workspace, or ~/.pudl only when --global names it explicitly, so
// running outside a repository never writes to ~/.pudl by accident.
func exampleInstallRoot(global bool) (string, error) {
	if !global {
		if wsPolicy == nil || !wsPolicy.InWorkspace() {
			return "", fmt.Errorf("example installation needs a workspace; run pudl init here, or pass --global to install into ~/.pudl")
		}
		return wsPolicy.Workspace.PudlDir, nil
	}
	root := wsPolicy.GlobalDir
	if _, err := os.Stat(config.ConfigPath(root)); err != nil {
		return "", fmt.Errorf("the global workspace %s is not initialized; run pudl init --global first", root)
	}
	return root, nil
}

func init() {
	rootCmd.AddCommand(exampleCmd)
	exampleCmd.AddCommand(exampleInstallCmd)
	exampleInstallCmd.Flags().Bool("overwrite", false, "Replace files that differ from this version, keeping yours as <file>.bak")
}

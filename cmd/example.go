package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/examples"
)

var exampleCmd = &cobra.Command{
	Use:   "example",
	Short: "Install bundled tutorials into this workspace",
}

var exampleInstallCmd = &cobra.Command{
	Use:       "install <name>",
	Short:     "Install a runnable tutorial and its fixture data",
	ValidArgs: examples.Names(),
	Args:      cobra.ExactArgs(1),
	Long: `Install a bundled tutorial in the current initialized PUDL workspace,
or with --global in the personal ~/.pudl workspace (after pudl init --global).
No source checkout or download is needed. Content packs (gcp, aws) install
with pudl pack install.

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

Example:
    pudl init
    pudl example install git-inventory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wroteJSON, err := installFileset(cmd, examples.Kind, args[0], examples.Install)
		if err != nil || wroteJSON {
			return err
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

func init() {
	rootCmd.AddCommand(exampleCmd)
	exampleCmd.AddCommand(exampleInstallCmd)
	exampleInstallCmd.Flags().Bool("overwrite", false, "Replace files that differ from this version, keeping yours as <file>.bak")
}

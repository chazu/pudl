package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/examples"
)

var exampleCmd = &cobra.Command{
	Use:   "example",
	Short: "Install bundled examples into this workspace",
}

var exampleInstallCmd = &cobra.Command{
	Use:       "install <name>",
	Short:     "Install a runnable example and its fixture data",
	ValidArgs: []string{"git-inventory"},
	Args:      cobra.ExactArgs(1),
	Long: `Install a bundled example in the current initialized PUDL workspace.
No source checkout or download is needed. Identical files are left alone;
existing files with different content are preserved and reported as conflicts.

The git-inventory example includes a model expecting default branch main,
baseline and changed observations, and an optional live observer. The import
and catalog replay tutorial needs only PUDL; live observation also needs mu
and Python 3.

Example:
    pudl repo init
    pudl example install git-inventory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if wsPolicy == nil || !wsPolicy.InWorkspace() {
			return fmt.Errorf("example installation needs a repository workspace; run pudl repo init first")
		}
		files, err := examples.Install(wsPolicy.Workspace.PudlDir, args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return GetOutputWriter().WriteJSON(map[string]any{"example": args[0], "root": wsPolicy.Workspace.PudlDir, "files": files})
		}
		fmt.Printf("Installed %s in %s\n", args[0], wsPolicy.Workspace.PudlDir)
		fmt.Printf("Model: git-inventory\nObservations: .pudl/populators/git-inventory/{baseline,changed}-observe.json\n")
		fmt.Println("Next: pudl model show git-inventory")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exampleCmd)
	exampleCmd.AddCommand(exampleInstallCmd)
}

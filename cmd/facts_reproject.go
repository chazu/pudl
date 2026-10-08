package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
)

var factsReprojectDryRun bool

var factsReprojectCmd = &cobra.Command{
	Use:   "reproject",
	Short: "Bring schema-projected facts in line with the catalog",
	Long: `Recompute the facts schemas project from imported records (_pudl.facts).

Imports and runs project facts as records arrive and sync afterwards, so this
is rarely needed by hand. It repairs, as corrections (retractions):

- resources whose facts describe an entry that was deleted, pruned,
  re-identified or reassigned to another schema;
- resources projected with an older facts block (the block changed);
- resources never projected (a facts block added after the data arrived).

A schema whose facts block is invalid is reported and its existing facts are
left unchanged. pudl query warns when a reprojection is pending.

Examples:
    pudl facts reproject
    pudl facts reproject --dry-run`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.NewCatalogDB(effectivePudlDir())
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()
		report, err := syncProjections(cmd.Context(), db, factsReprojectDryRun)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(map[string]any{"dry_run": factsReprojectDryRun, "report": report})
		}
		if factsReprojectDryRun {
			fmt.Fprintln(outw(), "Dry run: nothing written.")
		}
		printSyncReport(outw(), report, true)
		return nil
	},
}

func init() {
	factsCmd.AddCommand(factsReprojectCmd)
	factsReprojectCmd.Flags().BoolVar(&factsReprojectDryRun, "dry-run", false, "Report what would change without writing")
}

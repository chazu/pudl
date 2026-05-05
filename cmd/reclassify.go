package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
)

var reclassifyRef string

// reclassifyCmd retries classification for catalog rows whose declared
// schema reference was unresolved at import time. Today this is a
// status-reporting stub: a future change will look up the ref in the
// schema cache and, on hit, upgrade the row to status=declared.
var reclassifyCmd = &cobra.Command{
	Use:   "reclassify",
	Short: "Retry classification for items with unresolved schema references",
	Long: `Find items in the catalog tagged with an unresolved CUE schema reference
(schema ref declared at import time but not yet known to pudl) and
attempt to upgrade their classification.

When run without --ref, every unresolved row is considered. With --ref,
only rows tagged with that exact "<module>@<version>" reference are
processed.

This command is part of the mu plugin output schema flow — see
docs/plans/2026-05-04-feat-plugin-output-schemas-plan.md (W5).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReclassify(reclassifyRef)
	},
}

func init() {
	rootCmd.AddCommand(reclassifyCmd)
	reclassifyCmd.Flags().StringVar(&reclassifyRef, "ref", "", "Only reclassify rows tagged with this schema ref (e.g. mu/aws@v1)")
}

func runReclassify(ref string) error {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return fmt.Errorf("open catalog: %w", err)
	}
	defer db.Close()

	rows, err := db.ListUnresolvedItemSchemas(ref)
	if err != nil {
		return fmt.Errorf("list unresolved: %w", err)
	}
	if len(rows) == 0 {
		fmt.Println("No unresolved schema references found.")
		return nil
	}

	upgraded := 0
	stillUnresolved := 0
	for _, r := range rows {
		// Schema-cache lookup is not yet wired (cross-process exposure
		// of mu's schema cache to pudl is a follow-up). For now report
		// the unresolved set so operators know what's pending.
		resolved, err := tryResolveSchemaRef(r.SchemaRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  WARN %s -> %s: %v\n", r.ItemID, r.SchemaRef, err)
			stillUnresolved++
			continue
		}
		if !resolved {
			stillUnresolved++
			continue
		}
		if err := db.AddItemSchema(database.ItemSchema{
			ItemID:    r.ItemID,
			SchemaRef: r.SchemaRef,
			Status:    database.ItemSchemaStatusDeclared,
		}); err != nil {
			return fmt.Errorf("upgrade %s: %w", r.ItemID, err)
		}
		upgraded++
	}

	fmt.Printf("Reclassify: %d upgraded, %d still unresolved (of %d total).\n",
		upgraded, stillUnresolved, len(rows))
	if stillUnresolved > 0 {
		fmt.Println("Tip: schemas referenced by these rows are not yet known to pudl.")
		fmt.Println("Once the mu plugin schema cache is exposed to pudl, rerun `pudl reclassify`.")
	}
	return nil
}

// tryResolveSchemaRef attempts to resolve a schema reference against
// pudl's available schemas. Returns (true, nil) if the ref is now
// known and the row can be upgraded; (false, nil) if it is still
// unknown; or an error for unexpected lookup failures.
//
// Currently a stub — schema-cache lookup wiring is a follow-up.
func tryResolveSchemaRef(ref string) (bool, error) {
	_ = ref
	return false, nil
}

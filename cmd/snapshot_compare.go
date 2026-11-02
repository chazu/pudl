package cmd

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inventory"
)

var snapshotCompare string

func compareSnapshots(ctx context.Context, db *database.CatalogDB, before, after string) error {
	cmp, err := inventory.CompareSnapshots(ctx, db, before, after, invocationWorkspace().InventoryPolicy)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(map[string]any{"before_snapshot": cmp.Before, "after_snapshot": cmp.After, "changes": cmp.Changes, "identity_fallbacks": cmp.IdentityFallbacks})
	}
	fmt.Fprintf(outw(), "%s → %s: %d changes (complete: %t → %t)\n", before, after, len(cmp.Changes), cmp.Before.Complete, cmp.After.Complete)
	for _, note := range acute.FallbackNotes(cmp.IdentityFallbacks) {
		fmt.Fprintf(outw(), "  identity fallback: %s\n", note)
	}
	for _, change := range cmp.Changes {
		fmt.Fprintf(outw(), "  %s: %s\n", change.Resource, change.Change)
	}
	return nil
}

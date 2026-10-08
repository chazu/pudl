package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/projection"
)

// projectionRegistry resolves `_pudl.facts` specs over the active schema paths.
func projectionRegistry() (*projection.Registry, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("init schema inferrer: %w", err)
	}
	return projection.NewRegistry(inferrer, nil), nil
}

// loadEntryPayload decodes an entry's stored payload for re-projection.
func loadEntryPayload(entry database.CatalogEntry) (any, error) {
	return loadReinferData(entry.StoredPath, entry.Format)
}

// syncProjections reconciles projected facts with the catalog and current
// specs (see projection.Sync). Commands that change entries, schemas or
// identity call it when they finish; `pudl run` calls it before checks.
func syncProjections(ctx context.Context, db *database.CatalogDB, dryRun bool) (projection.SyncReport, error) {
	reg, err := projectionRegistry()
	if err != nil {
		return projection.SyncReport{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return projection.Sync(ctx, db, reg, loadEntryPayload, dryRun)
}

// syncProjectionsQuietly runs a sync after a write command and reports only
// what changed or failed, on stderr. Failure to sync never fails the command
// that already succeeded; `pudl facts reproject` repeats it explicitly.
func syncProjectionsQuietly(ctx context.Context, db *database.CatalogDB) {
	report, err := syncProjections(ctx, db, false)
	if err != nil {
		fmt.Fprintf(errw(), "⚠️  projected facts not synced: %v (run: pudl facts reproject)\n", err)
		return
	}
	printSyncReport(errw(), report, false)
}

// printSyncReport writes a sync report's notable parts.
func printSyncReport(w io.Writer, report projection.SyncReport, always bool) {
	if always || report.Changed() {
		fmt.Fprintf(w, "📐 projected facts: %d bootstrapped, %d reprojected, %d orphaned sources closed\n",
			report.Bootstrapped, report.Reprojected, report.Orphaned)
	}
	for schema, reason := range report.Broken {
		fmt.Fprintf(w, "⚠️  projection disabled for %s (existing facts unchanged): %s\n", schema, reason)
	}
	for _, e := range report.Errors {
		fmt.Fprintf(w, "⚠️  projection failed for %s\n", e)
	}
}

// warnProjectionHealth prints, on stderr and without writing, why projected
// facts may not reflect the catalog, and lint warnings for the rules the given
// relations depend on. Read-only commands (`pudl query`) use it instead of
// syncing.
func warnProjectionHealth(db *database.CatalogDB, rules []datalog.Rule, roots []string) {
	reg, err := projectionRegistry()
	if err != nil {
		return
	}
	if reasons, err := projection.Stale(db, reg); err == nil {
		for _, reason := range reasons {
			fmt.Fprintf(errw(), "⚠️  %s (run: pudl facts reproject)\n", reason)
		}
	}
	lintRules(db, reg, rules, roots)
}

// lintRules prints projection.Lint warnings on stderr.
func lintRules(db *database.CatalogDB, reg *projection.Registry, rules []datalog.Rule, roots []string) {
	stored := map[string]bool{}
	if relations, err := db.GetDistinctRelations(); err == nil {
		for _, r := range relations {
			stored[r] = true
		}
	}
	for _, w := range projection.Lint(rules, roots, reg.RelationArgs(), stored) {
		fmt.Fprintf(errw(), "⚠️  %s\n", w)
	}
}

// syncProjectionsAfterWrite opens the active catalog and syncs projected
// facts, for write commands that do not otherwise hold the catalog open.
func syncProjectionsAfterWrite(ctx context.Context) {
	db, err := database.NewCatalogDB(effectivePudlDir())
	if err != nil {
		fmt.Fprintf(errw(), "⚠️  projected facts not synced: %v (run: pudl facts reproject)\n", err)
		return
	}
	defer db.Close()
	syncProjectionsQuietly(ctx, db)
}

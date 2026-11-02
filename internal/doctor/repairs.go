package doctor

import (
	"context"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/repair"
)

// RepairRun is one pass of catalog repairs: diagnose the selected repairs, or
// apply them with Fix (previewed with DryRun).
type RepairRun struct {
	Selected []repair.Repair
	// PudlDir is the persistence root holding the catalog.
	PudlDir string
	Fix     bool
	DryRun  bool
	// HealthOK is whether the workspace health checks passed; Fix refuses to
	// write when they did not.
	HealthOK bool
	// Env builds what the repairs need over the opened catalog.
	Env func(db *database.CatalogDB) (*repair.Env, error)
	// AfterChange runs when a repair other than projections changed entries,
	// so projected facts follow them.
	AfterChange func(ctx context.Context, db *database.CatalogDB)
}

// RunRepairs runs the selected repairs and returns their results in order.
// It never creates a catalog: without one every repair is ok with "no
// catalog". A failure to open the catalog or build the environment is every
// repair's error result, not a failure of the run.
func RunRepairs(ctx context.Context, run RepairRun) []repair.Result {
	out := make([]repair.Result, 0, len(run.Selected))
	all := func(status, message string) []repair.Result {
		for _, r := range run.Selected {
			out = append(out, repair.Result{Name: r.Name, Condition: r.Condition, Status: status, Message: message})
		}
		return out
	}
	if _, err := os.Stat(filepath.Join(run.PudlDir, "data", "sqlite", "catalog.db")); os.IsNotExist(err) {
		return all(repair.StatusOK, "no catalog")
	}
	if run.Fix && !run.HealthOK {
		return all(repair.StatusSkipped, "workspace health checks failed; resolve them before applying repairs")
	}
	db, err := openRepairCatalog(run.PudlDir, run.Fix)
	if err != nil {
		return all(repair.StatusError, err.Error())
	}
	defer db.Close()
	env, err := run.Env(db)
	if err != nil {
		return all(repair.StatusError, err.Error())
	}
	changed, projected := false, false
	for _, r := range run.Selected {
		result := repair.Run(ctx, r, env, run.Fix, run.DryRun)
		changed = changed || result.Changed > 0
		projected = projected || r.Name == "projections"
		out = append(out, result)
	}
	if changed && !projected && run.AfterChange != nil {
		// Changed entries leave projected facts behind, as the former
		// standalone repair commands did.
		run.AfterChange(ctx, db)
	}
	return out
}

func openRepairCatalog(pudlDir string, fix bool) (*database.CatalogDB, error) {
	if fix {
		return database.NewCatalogDB(pudlDir)
	}
	return database.OpenCatalogDBReadOnly(pudlDir)
}

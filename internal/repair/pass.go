package repair

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/chazu/pudl/internal/database"
)

// Pass is one pass of `pudl doctor` over the catalog: diagnose the selected
// repairs, or apply them with Fix (previewed with DryRun).
type Pass struct {
	Selected []Repair
	// PudlDir is the persistence root holding the catalog.
	PudlDir string
	Fix     bool
	DryRun  bool
	// Only names the repairs the operator asked for. --fix applies an opt-in
	// repair only when Only names it.
	Only []string
	// HealthOK is whether the workspace health checks passed; Fix refuses to
	// write when they did not.
	HealthOK bool
	// Env builds what the repairs need over the opened catalog.
	Env func(db *database.CatalogDB) (*Env, error)
	// AfterChange runs when a repair other than projections changed entries,
	// so projected facts follow them.
	AfterChange func(ctx context.Context, db *database.CatalogDB)
}

// RunPass runs the selected repairs and returns their results in order.
// It never creates a catalog: without one every repair is ok with "no
// catalog". A failure to open the catalog or build the environment is every
// repair's error result, not a failure of the run.
func RunPass(ctx context.Context, run Pass) []Result {
	out := make([]Result, 0, len(run.Selected))
	all := func(status, message string) []Result {
		for _, r := range run.Selected {
			out = append(out, Result{Name: r.Name, Condition: r.Condition, Status: status, Message: message})
		}
		return out
	}
	if _, err := os.Stat(filepath.Join(run.PudlDir, "data", "sqlite", "catalog.db")); os.IsNotExist(err) {
		return all(StatusOK, "no catalog")
	}
	if run.Fix && !run.HealthOK {
		return all(StatusSkipped, "workspace health checks failed; resolve them before applying repairs")
	}
	db, err := openRepairCatalog(run.PudlDir, run.Fix)
	if err != nil {
		return all(StatusError, err.Error())
	}
	defer db.Close()
	env, err := run.Env(db)
	if err != nil {
		return all(StatusError, err.Error())
	}
	changed, projected := false, false
	for _, r := range run.Selected {
		if run.Fix && r.OptIn && !slices.Contains(run.Only, r.Name) {
			out = append(out, Result{Name: r.Name, Condition: r.Condition, Status: StatusSkipped,
				Message: "opt-in: name it with --only " + r.Name})
			continue
		}
		result := Run(ctx, r, env, run.Fix, run.DryRun)
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

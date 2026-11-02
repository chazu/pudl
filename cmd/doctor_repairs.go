package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/repair"
)

// Doctor repair flags.
var (
	doctorFix             bool
	doctorDryRun          bool
	doctorOnly            []string
	doctorSchema          string
	doctorOrigin          string
	doctorIncludeExplicit bool
)

// maxListedChanges bounds the changes listed per repair in text output; JSON
// lists every change.
const maxListedChanges = 20

// doctorRepair is one repair's result plus the commands that preview and
// apply it.
type doctorRepair struct {
	repair.Result
	Preview []string `json:"preview_argv,omitempty"`
	Apply   []string `json:"apply_argv,omitempty"`
}

// doctorMode names what doctor does with repairs: diagnose, dry-run or fix.
func doctorMode() string {
	switch {
	case doctorFix && doctorDryRun:
		return "dry-run"
	case doctorFix:
		return "fix"
	default:
		return "diagnose"
	}
}

// runDoctorRepairs diagnoses the selected repairs, or applies them with --fix.
// It never creates a catalog, and refuses to write when health checks failed.
func runDoctorRepairs(ctx context.Context, selected []repair.Repair, healthOK bool, catalog []catalogDiagnostic, scanned bool) []doctorRepair {
	out := make([]doctorRepair, 0, len(selected))
	all := func(status, message string) []doctorRepair {
		for _, r := range selected {
			out = append(out, doctorRepair{Result: repair.Result{Name: r.Name, Condition: r.Condition, Status: status, Message: message}})
		}
		return out
	}
	if _, err := os.Stat(filepath.Join(effectivePudlDir(), "data", "sqlite", "catalog.db")); os.IsNotExist(err) {
		return all(repair.StatusOK, "no catalog")
	}
	if doctorFix && !healthOK {
		return all(repair.StatusSkipped, "workspace health checks failed; resolve them before applying repairs")
	}
	db, err := openDoctorCatalog()
	if err != nil {
		return all(repair.StatusError, err.Error())
	}
	defer db.Close()
	env, err := repairEnv(db, catalog, scanned)
	if err != nil {
		return all(repair.StatusError, err.Error())
	}
	changed, projected := false, false
	for _, r := range selected {
		if doctorFix && r.OptIn && !slices.Contains(doctorOnly, r.Name) {
			out = append(out, doctorRepair{Result: repair.Result{Name: r.Name, Condition: r.Condition, Status: repair.StatusSkipped,
				Message: "opt-in: name it with --only " + r.Name}})
			continue
		}
		result := repair.Run(ctx, r, env, doctorFix, doctorDryRun)
		changed = changed || result.Changed > 0
		projected = projected || r.Name == "projections"
		entry := doctorRepair{Result: result}
		if result.Status == repair.StatusNeeded || result.Status == repair.StatusPartial {
			entry.Preview, entry.Apply = repairArgv(r.Name, true), repairArgv(r.Name, false)
		}
		out = append(out, entry)
	}
	if changed && !projected {
		// Changed entries leave projected facts behind, as the former
		// standalone repair commands did.
		syncProjectionsQuietly(ctx, db)
	}
	return out
}

func openDoctorCatalog() (*database.CatalogDB, error) {
	if doctorFix {
		return database.NewCatalogDB(effectivePudlDir())
	}
	return database.OpenCatalogDBReadOnly(effectivePudlDir())
}

func repairEnv(db *database.CatalogDB, catalog []catalogDiagnostic, scanned bool) (*repair.Env, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, err
	}
	inferrer, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return nil, err
	}
	reg, err := projectionRegistry()
	if err != nil {
		return nil, err
	}
	opts := repair.Options{
		DriftScanned: scanned, IncludeExplicit: doctorIncludeExplicit,
		EntryID: doctorEntry, Schema: doctorSchema, Origin: doctorOrigin,
	}
	for _, entry := range catalog {
		if entry.Status == "mismatch" {
			opts.DriftEntryIDs = append(opts.DriftEntryIDs, entry.ID)
		}
	}
	return &repair.Env{DB: db, Inferrer: inferrer, Projections: reg, Options: opts}, nil
}

// repairArgv is the doctor invocation that previews (or applies) one repair
// with the current scope.
func repairArgv(name string, preview bool) []string {
	argv := []string{"pudl", "doctor", "--fix"}
	if preview {
		argv = append(argv, "--dry-run")
	}
	argv = append(argv, "--only", name)
	scope := [][2]string{{"--entry", doctorEntry}, {"--schema", doctorSchema}, {"--origin", doctorOrigin}}
	for _, flag := range scope {
		if flag[1] != "" {
			argv = append(argv, flag[0], flag[1])
		}
	}
	if doctorIncludeExplicit {
		argv = append(argv, "--include-explicit")
	}
	return argv
}

// repairsChanged reports whether any repair wrote something.
func repairsChanged(repairs []doctorRepair) bool {
	for _, r := range repairs {
		if r.Changed > 0 {
			return true
		}
	}
	return false
}

// repairsFailed reports whether an applied repair could not complete.
func repairsFailed(repairs []doctorRepair) bool {
	for _, r := range repairs {
		if r.Status == repair.StatusError || r.Status == repair.StatusPartial {
			return true
		}
	}
	return false
}

func printDoctorRepairs(repairs []doctorRepair) {
	for _, r := range repairs {
		fmt.Fprintf(outw(), "Repair %s: %s — %s\n", r.Name, r.Status, r.Message)
		for i, change := range r.Changes {
			if i == maxListedChanges {
				fmt.Fprintf(outw(), "  ... %d more (see --json)\n", len(r.Changes)-maxListedChanges)
				break
			}
			fmt.Fprintf(outw(), "  %s: %s -> %s\n", change.Subject, change.From, change.To)
		}
		for _, e := range r.Errors {
			fmt.Fprintf(outw(), "  error: %s\n", e)
		}
		if len(r.Apply) > 0 {
			fmt.Fprintf(outw(), "  preview: %s\n  apply:   %s\n", strings.Join(r.Preview, " "), strings.Join(r.Apply, " "))
		}
	}
	if doctorMode() == "dry-run" {
		fmt.Fprintln(outw(), "Dry run: no repairs written.")
	}
}

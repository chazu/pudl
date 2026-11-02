package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/doctor"
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

// runDoctorRepairs diagnoses the selected repairs, or applies them with --fix,
// and names the commands that preview and apply each one still needed.
func runDoctorRepairs(ctx context.Context, selected []repair.Repair, healthOK bool, catalog []doctor.CatalogDiagnostic, scanned bool) []doctorRepair {
	results := repair.RunPass(ctx, repair.Pass{
		Selected: selected, PudlDir: effectivePudlDir(),
		Fix: doctorFix, DryRun: doctorDryRun, Only: doctorOnly, HealthOK: healthOK,
		Env: func(db *database.CatalogDB) (*repair.Env, error) {
			return repairEnv(db, catalog, scanned)
		},
		AfterChange: syncProjectionsQuietly,
	})
	out := make([]doctorRepair, 0, len(results))
	for _, result := range results {
		entry := doctorRepair{Result: result}
		if result.Status == repair.StatusNeeded || result.Status == repair.StatusPartial {
			entry.Preview, entry.Apply = repairArgv(result.Name, true), repairArgv(result.Name, false)
		}
		out = append(out, entry)
	}
	return out
}

func repairEnv(db *database.CatalogDB, catalog []doctor.CatalogDiagnostic, scanned bool) (*repair.Env, error) {
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
		DriftEntryIDs: doctor.DriftEntryIDs(catalog),
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

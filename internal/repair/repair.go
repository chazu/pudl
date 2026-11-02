// Package repair holds the catalog maintenance repairs `pudl doctor` diagnoses
// and `pudl doctor --fix` applies. Each repair detects one condition and fixes
// it; diagnosis never writes.
//
// Adding a repair is one file defining a Repair value plus one line in
// Registry, placed where its ordering dependencies are met.
package repair

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/projection"
)

// Result statuses.
const (
	StatusOK      = "ok"      // nothing to repair
	StatusNeeded  = "needed"  // diagnosis or dry run found something to repair
	StatusApplied = "applied" // the repair wrote every change it found
	StatusPartial = "partial" // some changes failed; see Errors
	StatusError   = "error"   // the repair could not run
	StatusSkipped = "skipped" // not evaluated; Message says why
)

// Repair detects one catalog condition and fixes it.
type Repair struct {
	// Name selects the repair with `pudl doctor --fix --only NAME`.
	Name string
	// Condition describes, in one line, what the repair detects.
	Condition string
	// Diagnose reports what the repair would change. It must not write, and
	// it may run against a read-only catalog.
	Diagnose func(ctx context.Context, env *Env) (Result, error)
	// Apply makes the changes, or with dryRun reports them without writing.
	// It needs a writable catalog.
	Apply func(ctx context.Context, env *Env, dryRun bool) (Result, error)
	// OptIn repairs are diagnosed like any other, but `--fix` applies them
	// only when `--only` names them: naming one is the confirmation.
	OptIn bool
}

// Registry lists every repair in the order `pudl doctor --fix` applies them:
// names are canonical before classification, classification settles before
// identity is computed from it, and projected facts follow the final entries.
// Policy leaves move to their types before inference re-checks classification.
// Data permissions depend on no other repair. The opt-in sensitive scrub runs
// last, against the final classification, and re-projects what it rewrites.
func Registry() []Repair {
	return []Repair{schemaNames, policyLeaves, inferenceDrift, identities, projections, dataPermissions, sensitiveScrub}
}

// Names returns the registered repair names in order.
func Names() []string {
	var names []string
	for _, r := range Registry() {
		names = append(names, r.Name)
	}
	return names
}

// Select returns the registered repairs named in only, in registry order, or
// every repair when only is empty.
func Select(only []string) ([]Repair, error) {
	all := Registry()
	if len(only) == 0 {
		return all, nil
	}
	wanted := map[string]bool{}
	for _, name := range only {
		wanted[name] = true
	}
	var selected []Repair
	for _, r := range all {
		if wanted[r.Name] {
			selected = append(selected, r)
			delete(wanted, r.Name)
		}
	}
	for name := range wanted {
		return nil, fmt.Errorf("unknown repair %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return selected, nil
}

// Env is what repairs run against.
type Env struct {
	DB          *database.CatalogDB
	Inferrer    *inference.SchemaInferrer
	Projections *projection.Registry
	Options     Options
}

// Options are the low-level recovery controls of individual repairs.
type Options struct {
	// DriftEntryIDs are the entries doctor's catalog scan found whose
	// inferred schema differs from the assigned one. DriftScanned reports
	// whether that scan ran.
	DriftEntryIDs []string
	DriftScanned  bool
	// IncludeExplicit makes inference re-infer every entry in scope (EntryID,
	// or Schema and Origin), including explicit assignments and entries that
	// failed validation, instead of only the drift doctor found.
	IncludeExplicit bool
	EntryID         string
	Schema          string
	Origin          string
}

// Result is what a repair found and, when applied, changed.
type Result struct {
	Name      string   `json:"name"`
	Condition string   `json:"condition"`
	Status    string   `json:"status"`
	Message   string   `json:"message"`
	Pending   int      `json:"pending"`
	Changed   int      `json:"changed"`
	Failed    int      `json:"failed"`
	Changes   []Change `json:"changes,omitempty"`
	Errors    []string `json:"errors,omitempty"`
	// Details carries repair-specific structured output, such as the
	// projection sync report.
	Details any `json:"details,omitempty"`
}

// Change is one subject a repair changed or would change.
type Change struct {
	Subject string `json:"subject"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
}

// Run diagnoses r, or applies it when apply is set (writing nothing when
// dryRun is also set), and fills in the result's name, condition and status.
func Run(ctx context.Context, r Repair, env *Env, apply, dryRun bool) Result {
	var result Result
	var err error
	if apply {
		result, err = r.Apply(ctx, env, dryRun)
	} else {
		result, err = r.Diagnose(ctx, env)
	}
	result.Name, result.Condition = r.Name, r.Condition
	switch {
	case err != nil:
		result.Status, result.Message = StatusError, err.Error()
	case result.Status != "":
	case apply && result.Failed > 0:
		result.Status = StatusPartial
	case apply && !dryRun && result.Changed > 0:
		result.Status = StatusApplied
	case result.Pending > 0:
		result.Status = StatusNeeded
	default:
		result.Status = StatusOK
	}
	return result
}

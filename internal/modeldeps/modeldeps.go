// Package modeldeps maintains the cross-model dependency graph: the
// model_depends_on facts a model's declared depends_on reconciles into, and
// the read-only upstream-freshness advisory over them. See
// docs/cross-model-dependencies.md.
package modeldeps

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/systemmodel"
)

// Relation is the EDB relation emitted from a model's declared depends_on.
// Arg keys are the load-bearing contract shared by the shipped recursive rules
// (depends_transitive / impacted_by / cyclic) and `pudl query`:
// {from: <declaring model>, to: <dependency>}.
const Relation = "model_depends_on"

// DeclaredSource is the fact Source of a model's declared edges. Source tags
// partition model_depends_on edges by provenance so the declared reconcile and
// any derived reconcile never clobber each other: each only manages edges
// carrying its own source.
func DeclaredSource(model string) string { return "model:" + model }

// Resolver canonicalizes a dependency name to the instance name of the model
// it resolves to, or reports that it resolves to none.
type Resolver func(name string) (canonical string, ok bool)

// RegisteredIn resolves dependency names against the models registered in
// the schema dirs (see systemmodel.ResolveTemplate).
func RegisteredIn(dirs []string) Resolver {
	return func(name string) (string, bool) {
		found, err := systemmodel.ResolveTemplate(dirs, name)
		if err != nil || found == nil {
			return "", false
		}
		return found.Name, true
	}
}

// Declared canonicalizes a model's declared depends_on into a set of resolved
// instance names (so the `to` recorded matches the `from` that dependency
// records for its own edges) and returns warnings for self-dependencies and
// dependencies that do not resolve to a known model. An unresolved edge is
// still recorded — forward references register — but impact answers stay
// partial until it exists.
func Declared(model string, dependencies []string, resolve Resolver) (map[string]struct{}, []string) {
	declared := make(map[string]struct{}, len(dependencies))
	var warnings []string
	for _, dep := range dependencies {
		if dep == "" {
			continue
		}
		if dep == model {
			warnings = append(warnings, fmt.Sprintf("model %q declares a dependency on itself; ignored", model))
			continue
		}
		canonical := dep
		if found, ok := resolve(dep); ok {
			canonical = found
		} else {
			warnings = append(warnings, fmt.Sprintf("depends_on %q does not resolve to a known model (edge recorded; impact answers stay partial until it is registered/run)", dep))
		}
		declared[canonical] = struct{}{}
	}
	return declared, warnings
}

// Reconcile brings the model_depends_on edges for one model and source in
// line with the wanted set. It is a diff, not a blind append:
//
//   - wanted edge not currently valid  -> AddFact
//   - currently-valid edge not wanted  -> InvalidateFact (valid-time end)
//   - wanted edge already valid        -> no-op
//
// Scoping by Source keeps re-runs idempotent (no per-run fact churn — AddFact's
// valid_start defaults to now, so a blind re-add would mint a new fact every
// run) and keeps the declared graph and any derived graph independent. Edges
// are keyed on the instance name, so model-level names join cleanly across the
// closure.
func Reconcile(db *database.CatalogDB, from, source string, wanted map[string]struct{}) error {
	return db.WithCatalogTx(func(tx *database.CatalogTx) error {
		return reconcileIn(tx, from, source, wanted)
	})
}

type factStore interface {
	QueryFacts(filter database.FactFilter) ([]database.Fact, error)
	AddFact(fact database.Fact) (database.Fact, error)
	InvalidateFact(id string) error
}

func reconcileIn(store factStore, from, source string, wanted map[string]struct{}) error {
	facts, err := store.QueryFacts(database.FactFilter{Relation: Relation})
	if err != nil {
		return err
	}
	current := make(map[string]string) // to -> fact ID, scoped to this source+from
	for _, f := range facts {
		if f.Source != source {
			continue
		}
		ff, to := EdgeArgs(f.Args)
		if ff == from && to != "" {
			current[to] = f.ID
		}
	}

	add, invalidate := Diff(wanted, current)
	for _, id := range invalidate {
		if ierr := store.InvalidateFact(id); ierr != nil {
			return fmt.Errorf("invalidate stale dependency for %s: %w", from, ierr)
		}
	}
	for _, to := range add {
		args, merr := json.Marshal(map[string]string{"from": from, "to": to})
		if merr != nil {
			return merr
		}
		if _, aerr := store.AddFact(database.Fact{
			Relation: Relation,
			Args:     string(args),
			Source:   source,
		}); aerr != nil {
			return fmt.Errorf("add dependency %s->%s: %w", from, to, aerr)
		}
	}
	return nil
}

// Diff computes the reconcile plan: the dependency targets to add (declared
// but not currently valid) and the fact IDs to invalidate (currently valid but
// no longer declared). An edge present in both is a no-op — this is what keeps
// re-runs idempotent (no per-run fact churn). Both lists are sorted for
// deterministic application and output.
func Diff(declared map[string]struct{}, current map[string]string) (add []string, invalidate []string) {
	for to, id := range current {
		if _, ok := declared[to]; !ok {
			invalidate = append(invalidate, id)
		}
	}
	for to := range declared {
		if _, ok := current[to]; !ok {
			add = append(add, to)
		}
	}
	sort.Strings(add)
	sort.Strings(invalidate)
	return add, invalidate
}

// StaleUpstreams is a read-only advisory: it warns when any model this one
// transitively depends on is itself `drifted` or `failed` (a stale-input
// guard). It evaluates depends_transitive over rules, then reads the
// per-target status of each upstream.
//
// Coverage caveat (see docs/cross-model-dependencies.md): edges exist only for
// upstreams that have been run, so silence is "no recorded stale upstream",
// not a proof of freshness. Best-effort: any failure returns no warnings,
// never an error — this never blocks a check.
func StaleUpstreams(db *database.CatalogDB, rules []datalog.Rule, model string) []string {
	ups, err := datalog.Evaluate(db, rules, "depends_transitive",
		map[string]interface{}{"from": model}, datalog.TemporalScope{})
	if err != nil {
		return nil
	}
	if len(ups) == 0 {
		return nil
	}

	statuses, err := db.GetTargetStatuses()
	if err != nil {
		return nil
	}
	statusByTarget := make(map[string]string, len(statuses))
	for _, s := range statuses {
		statusByTarget[s.Target] = s.Status
	}

	var stale []string
	seen := map[string]struct{}{}
	for _, t := range ups {
		to, ok := t.Args["to"].(string)
		if !ok {
			continue
		}
		if _, dup := seen[to]; dup {
			continue
		}
		seen[to] = struct{}{}
		switch statusByTarget[systemmodel.InstanceTarget(to)] {
		case "drifted", "failed":
			stale = append(stale, fmt.Sprintf("%s (%s)", to, statusByTarget[systemmodel.InstanceTarget(to)]))
		}
	}
	if len(stale) == 0 {
		return nil
	}
	sort.Strings(stale)
	return []string{fmt.Sprintf("upstream(s) not clean: %v — this model may be converging against stale inputs", stale)}
}

// EdgeArgs extracts the from/to of a model_depends_on fact's args JSON.
func EdgeArgs(argsJSON string) (from, to string) {
	var a map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", ""
	}
	if s, ok := a["from"].(string); ok {
		from = s
	}
	if s, ok := a["to"].(string); ok {
		to = s
	}
	return from, to
}

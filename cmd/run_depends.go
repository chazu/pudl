package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/systemmodel"
)

// modelDependsRelation is the EDB relation emitted from a model's declared
// depends_on. Arg keys are the load-bearing contract shared by the shipped
// recursive rules (depends_transitive / impacted_by / cyclic) and `pudl query`:
// {from: <declaring model>, to: <dependency>}. See docs/cross-model-dependencies.md.
const modelDependsRelation = "model_depends_on"

// reconcileModelDependencies brings the model_depends_on facts for this model in
// line with its declared depends_on, then returns any warnings (e.g. a dep that
// does not resolve to a known model). It is a DIFF, not a blind append:
//
//   - declared edge not currently valid  -> AddFact
//   - currently-valid edge not declared  -> InvalidateFact (valid-time end)
//   - declared edge already valid         -> no-op
//
// This keeps re-runs idempotent (no per-run fact churn — AddFact's valid_start
// defaults to now, so a blind re-add would mint a new fact every run) and keeps
// blast-radius answers truthful when an author removes a dependency.
//
// Edges are keyed on the instance NAME (never a //models/<name> target or a
// :populate sub-target), so model-level names join cleanly across the closure.
// A dep that does not resolve via resolveModel is still recorded (so forward
// references to a not-yet-created model register), but is reported as a warning.
func reconcileModelDependencies(m *systemmodel.SystemModel) (warnings []string, err error) {
	// Canonicalize the declared set: map each dep to the resolved instance name
	// so the `to` we record matches the `from` that dep records for its own
	// edges. Unresolved deps keep their literal name and earn a warning.
	declared := make(map[string]struct{}, len(m.DependsOn))
	for _, dep := range m.DependsOn {
		if dep == "" || dep == m.Name {
			if dep == m.Name {
				warnings = append(warnings, fmt.Sprintf("model %q declares a dependency on itself; ignored", m.Name))
			}
			continue
		}
		canonical := dep
		if found, _, _, rerr := resolveModel(dep); rerr == nil && found != nil {
			canonical = found.Name
		} else {
			warnings = append(warnings, fmt.Sprintf("depends_on %q does not resolve to a known model (edge recorded; impact answers stay partial until it is registered/run)", dep))
		}
		declared[canonical] = struct{}{}
	}

	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return warnings, err
	}
	defer db.Close()

	// Currently-valid edges originating at this model (from == m.Name).
	facts, err := db.QueryFacts(database.FactFilter{Relation: modelDependsRelation})
	if err != nil {
		return warnings, err
	}
	current := make(map[string]string) // to -> fact ID
	for _, f := range facts {
		from, to := edgeArgs(f.Args)
		if from == m.Name && to != "" {
			current[to] = f.ID
		}
	}

	add, invalidate := dependencyDiff(declared, current)

	// Removed: currently valid but no longer declared -> invalidate.
	for _, id := range invalidate {
		if ierr := db.InvalidateFact(id); ierr != nil {
			return warnings, fmt.Errorf("invalidate stale dependency for %s: %w", m.Name, ierr)
		}
	}

	// Added: declared but not currently valid -> add.
	for _, to := range add {
		args, merr := json.Marshal(map[string]string{"from": m.Name, "to": to})
		if merr != nil {
			return warnings, merr
		}
		if _, aerr := db.AddFact(database.Fact{
			Relation: modelDependsRelation,
			Args:     string(args),
			Source:   "model:" + m.Name,
		}); aerr != nil {
			return warnings, fmt.Errorf("add dependency %s->%s: %w", m.Name, to, aerr)
		}
	}

	return warnings, nil
}

// dependencyDiff computes the reconcile plan: the dependency targets to add
// (declared but not currently valid) and the fact IDs to invalidate (currently
// valid but no longer declared). An edge present in both is a no-op — this is
// what keeps re-runs idempotent (no per-run fact churn). Both lists are sorted
// for deterministic application/output.
func dependencyDiff(declared map[string]struct{}, current map[string]string) (add []string, invalidate []string) {
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

// checkUpstreamFreshness is a read-only advisory: it warns when any model this
// one transitively depends on is itself `drifted` or `failed` (a stale-input
// guard). It evaluates depends_transitive over the shipped rules, then reads the
// per-target status of each upstream.
//
// Coverage caveat (see docs/cross-model-dependencies.md): edges exist only for
// upstreams that have been run, so silence is "no recorded stale upstream", not
// a proof of freshness. Best-effort: any failure returns no warnings, never an
// error — this never blocks a run.
func checkUpstreamFreshness(m *systemmodel.SystemModel) []string {
	configDir := config.GetPudlDir()
	rules, err := loadQueryRules(configDir)
	if err != nil {
		return nil
	}
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil
	}
	defer db.Close()

	ups, err := datalog.Evaluate(db, rules, "depends_transitive",
		map[string]interface{}{"from": m.Name}, datalog.TemporalScope{})
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
		switch statusByTarget[modelTargetKey(to)] {
		case "drifted", "failed":
			stale = append(stale, fmt.Sprintf("%s (%s)", to, statusByTarget[modelTargetKey(to)]))
		}
	}
	if len(stale) == 0 {
		return nil
	}
	sort.Strings(stale)
	return []string{fmt.Sprintf("upstream(s) not clean: %v — this model may be converging against stale inputs", stale)}
}

// edgeArgs extracts the from/to of a model_depends_on fact's args JSON.
func edgeArgs(argsJSON string) (from, to string) {
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

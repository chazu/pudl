package cmd

import (
	"fmt"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/systemmodel"
)

// Check scopes recorded on a result. "run" means the check was evaluated with
// this run's ID bound as a constraint; "global" means it saw the whole catalog.
const (
	checkScopeRun    = "run"
	checkScopeGlobal = "global"
)

// CheckResult is the outcome of one model check (a Datalog relation evaluated
// over the catalog, asserted empty/nonempty).
//
// Count is the number of result rows the verdict is *about*: under `--only`, an
// `expect: empty` check excuses rows that name only excluded resources, and
// those are counted in AdvisoryCount instead. Without `--only` — and for
// `expect: nonempty` checks, which count evidence rather than violations —
// AdvisoryCount is zero and Count is the full result size.
type CheckResult struct {
	Name          string `json:"name"`
	Query         string `json:"query"`
	Severity      string `json:"severity"`
	Count         int    `json:"count"`
	AdvisoryCount int    `json:"advisory_count,omitempty"`
	Scope         string `json:"scope"`
	Passed        bool   `json:"passed"`
	Message       string `json:"message,omitempty"`
}

// checkContext is what a run knows that scopes its checks: the run's own ID,
// whether it observed anything under that ID, and its `--only` selection.
type checkContext struct {
	runID string
	// fromCatalog marks a replay. A replay observes nothing, so no catalog row
	// carries its run ID; binding the constraint would make every `expect: empty`
	// check pass trivially.
	fromCatalog bool
	scope       *acute.TupleScope
}

// checkPasses is the pure expect-vs-count verdict: "empty" passes on no tuples,
// "nonempty" passes on at least one.
func checkPasses(expect string, count int) bool {
	switch expect {
	case "empty":
		return count == 0
	case "nonempty":
		return count > 0
	default:
		return false
	}
}

// headExposesRunID reports whether any rule producing this relation declares
// `run_id` as a variable head argument.
//
// Constraints are applied as `WHERE "<key>" = ?` over the derived head columns,
// so passing a key the head does not expose is a SQL error rather than a wider
// query. Testing the head first makes run scoping opt-in by the rule author: a
// rule that wants it binds `catalog_entry(run_id: $R, …)` in its body and
// surfaces `$R` in its head; every other rule evaluates catalog-wide exactly as
// before.
func headExposesRunID(rules []datalog.Rule, relation string) bool {
	for _, rule := range rules {
		if rule.Head.Rel != relation {
			continue
		}
		if term, ok := rule.Head.Args["run_id"]; ok && term.IsVariable() {
			return true
		}
	}
	return false
}

// runChecks evaluates each of the model's checks (a Datalog relation over the
// catalog) and returns the per-check verdicts. Rules are loaded from the standard
// pudl paths plus the model's rules/ subdir.
func runChecks(cat *runCatalog, m *systemmodel.SystemModel, modelDir string, ctx checkContext) ([]CheckResult, error) {
	if len(m.Checks) == 0 {
		return nil, nil
	}
	db, err := cat.required()
	if err != nil {
		return nil, err
	}

	rules, err := datalog.LoadRulesFromPaths(rulePathsForModel(modelDir)...)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}

	var results []CheckResult
	for _, c := range m.Checks {
		scope := checkScopeGlobal
		var constraints map[string]interface{}
		if !ctx.fromCatalog && ctx.runID != "" && headExposesRunID(rules, c.Query) {
			scope = checkScopeRun
			constraints = map[string]interface{}{"run_id": ctx.runID}
		}

		tuples, err := datalog.Evaluate(db, rules, c.Query, constraints, datalog.TemporalScope{})
		if err != nil {
			return nil, fmt.Errorf("check %q (relation %q): %w", c.Name, c.Query, err)
		}

		gating, advisory := partitionCheckTuples(tuples, c.Expect, ctx.scope)
		results = append(results, CheckResult{
			Name:          c.Name,
			Query:         c.Query,
			Severity:      c.Severity,
			Count:         gating,
			AdvisoryCount: advisory,
			Scope:         scope,
			Passed:        checkPasses(c.Expect, gating),
			Message:       c.Message,
		})
	}
	return results, nil
}

// partitionCheckTuples splits a check's result rows into the ones its verdict is
// about and the ones `--only` excused.
//
// Only `expect: empty` checks partition. Such a check counts violations, so
// excusing violations on resources the run excluded is the point. An
// `expect: nonempty` check counts *evidence*, and dropping evidence could only
// manufacture a failure that nothing in scope can fix — so it gates on the full
// result set, as it did before scoping existed.
func partitionCheckTuples(tuples []datalog.Tuple, expect string, scope *acute.TupleScope) (gating, advisory int) {
	if expect != "empty" || !scope.Restricted() {
		return len(tuples), 0
	}
	for _, t := range tuples {
		if scope.Advisory(acute.ArgValues(t.Args)) {
			advisory++
			continue
		}
		gating++
	}
	return gating, advisory
}

// printChecks renders the check results and reports whether any check with
// severity "fail" did not pass (the caller turns that into a non-zero exit).
//
// A check that failed only outside the run's scope is rendered as advisory
// rather than FAIL: rendering FAIL while the exit code silently stayed zero is
// what trains people to ignore checks.
func printChecks(results []CheckResult) (failedFail bool) {
	for _, r := range results {
		switch {
		case r.Passed && r.AdvisoryCount > 0:
			fmt.Printf("  ⚠ %s [%s] advisory — %d match(es) outside --only scope: %s\n",
				r.Name, r.Severity, r.AdvisoryCount, r.Message)
		case r.Passed:
			fmt.Printf("  ✓ %s (%s)\n", r.Name, r.Severity)
		default:
			if r.Severity == "fail" {
				failedFail = true
			}
			outside := ""
			if r.AdvisoryCount > 0 {
				outside = fmt.Sprintf(" (+%d outside --only scope)", r.AdvisoryCount)
			}
			fmt.Printf("  ✗ %s [%s] FAIL — %d match(es)%s: %s\n",
				r.Name, r.Severity, r.Count, outside, r.Message)
		}
	}
	return failedFail
}

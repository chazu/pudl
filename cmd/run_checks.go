package cmd

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/evidence"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/systemmodel"
)

// Check scopes recorded on a result. "run" means the check was evaluated with
// this run's ID bound as a constraint; "global" means it saw the whole catalog.
const (
	checkScopeRun    = "run"
	checkScopeGlobal = "latest-known"
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
	Evidence           []evidence.Reference    `json:"evidence,omitempty"`
	Outcome            string                  `json:"outcome,omitempty"`
	Diagnostics        []projection.Diagnostic `json:"diagnostics,omitempty"`
	Expect             string                  `json:"expect,omitempty"`
	Witnesses          []CheckWitness          `json:"witnesses,omitempty"`
	WitnessesTruncated bool                    `json:"witnesses_truncated,omitempty"`
	Name               string                  `json:"name"`
	Query              string                  `json:"query"`
	Severity           string                  `json:"severity"`
	Count              int                     `json:"count"`
	AdvisoryCount      int                     `json:"advisory_count,omitempty"`
	Scope              string                  `json:"scope"`
	Passed             bool                    `json:"passed"`
	Message            string                  `json:"message,omitempty"`
}

// checkContext is what a run knows that scopes its checks: the run's own ID,
// whether it observed anything under that ID, and its `--only` selection.
type checkContext struct {
	currentSnapshot string
	runID           string
	// fromCatalog marks a replay. A replay observes nothing, so no catalog row
	// carries its run ID; binding the constraint would make every `expect: empty`
	// check pass trivially.
	fromCatalog bool
	scope       *acute.TupleScope
}

// checkPasses is the pure expect-vs-count verdict: "empty" passes on no tuples,
// "nonempty" passes on at least one.
func checkPasses(expect string, count int) bool { return checks.Passes(expect, count) }

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
	return checks.HeadExposesRunID(rules, relation)
}

// runChecks evaluates each of the model's checks (a Datalog relation over the
// catalog) and returns the per-check verdicts. Rules are loaded from the standard
// pudl paths plus the model's rules/ subdir.
func runChecks(cat *runCatalog, m *systemmodel.SystemModel, modelDir string, ctx checkContext) ([]CheckResult, error) {
	return runChecksContext(context.Background(), cat, m, modelDir, ctx)
}

func runChecksContext(evalCtx context.Context, cat *runCatalog, m *systemmodel.SystemModel, modelDir string, ctx checkContext) ([]CheckResult, error) {
	if len(m.Checks) == 0 {
		return nil, nil
	}
	db, err := cat.required()
	if err != nil {
		return nil, err
	}
	reg, err := projectionRegistry()
	if err != nil {
		return nil, err
	}
	evaluated, err := checks.Evaluate(evalCtx, checks.Request{Catalog: db, Registry: reg, RulePaths: rulePathsForModel(modelDir), Load: loadEntryPayload, Checks: m.Checks, RunID: ctx.runID, CurrentSnapshot: ctx.currentSnapshot, FromCatalog: ctx.fromCatalog, Scope: ctx.scope, Redact: func(value string) string { return redactSealedText(value, m) }, OnProgress: emitCheckProgress})
	if err != nil {
		return nil, err
	}
	results := make([]CheckResult, 0, len(evaluated))
	for _, e := range evaluated {
		witnesses, truncated := checkWitnesses(e.Tuples, e.Expect, ctx.scope, m)
		results = append(results, CheckResult{Name: e.Name, Query: e.Query, Expect: e.Expect, Severity: e.Severity, Message: e.Message, Scope: e.Scope, Outcome: e.Outcome, Passed: e.Passed, Count: e.Count, AdvisoryCount: e.AdvisoryCount, Evidence: e.Evidence, Diagnostics: e.Diagnostics, Witnesses: witnesses, WitnessesTruncated: truncated})
	}
	return results, nil
}

func partitionCheckTuples(tuples []datalog.Tuple, expect string, scope *acute.TupleScope) (int, int) {
	return checks.Partition(tuples, expect, scope)
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
		case r.Outcome == "unknown" || r.Outcome == "error":
			fmt.Fprintf(outw(), "  ? %s [%s]: %s\n", r.Name, r.Severity, r.Outcome)
			for _, d := range r.Diagnostics {
				fmt.Fprintf(outw(), "    %s: %s\n", d.Code, d.Message)
			}
			failedFail = true
		case r.Passed && r.AdvisoryCount > 0:
			fmt.Fprintf(outw(), "  ⚠ %s [%s] advisory — %d match(es) outside --only scope: %s\n",
				r.Name, r.Severity, r.AdvisoryCount, r.Message)
		case r.Passed:
			fmt.Fprintf(outw(), "  ✓ %s (%s)\n", r.Name, r.Severity)
		default:
			if r.Severity == "fail" {
				failedFail = true
			}
			outside := ""
			if r.AdvisoryCount > 0 {
				outside = fmt.Sprintf(" (+%d outside --only scope)", r.AdvisoryCount)
			}
			fmt.Fprintf(outw(), "  ✗ %s [%s] FAIL — %d match(es)%s: %s\n",
				r.Name, r.Severity, r.Count, outside, r.Message)
		}
		writeCheckFindings(outw(), r)
	}
	return failedFail
}

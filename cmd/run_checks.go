package cmd

import (
	"context"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/systemmodel"
)

// checkContext is what a run knows that scopes its checks: the run's own ID,
// and whether it observed anything under that ID.
type checkContext struct {
	currentSnapshot string
	runID           string
	// fromCatalog marks a replay. A replay observes nothing, so no catalog row
	// carries its run ID; binding the constraint would make every `expect: empty`
	// check pass trivially.
	fromCatalog bool
}

// runChecks evaluates each of the model's checks (a Datalog relation over the
// catalog) and returns the per-check verdicts. Rules are loaded from the standard
// pudl paths plus the model's rules/ subdir.
func runChecks(cat *runCatalog, m *systemmodel.SystemModel, modelDir string, ctx checkContext) ([]checkreport.CheckResult, error) {
	return runChecksContext(context.Background(), cat, m, modelDir, ctx)
}

func runChecksContext(evalCtx context.Context, cat *runCatalog, m *systemmodel.SystemModel, modelDir string, ctx checkContext) ([]checkreport.CheckResult, error) {
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
	evaluated, err := checks.Evaluate(evalCtx, checks.Request{Catalog: db, Registry: reg, RulePaths: rulePathsForModel(modelDir), Load: loadEntryPayload, Checks: m.Checks, RunID: ctx.runID, CurrentSnapshot: ctx.currentSnapshot, FromCatalog: ctx.fromCatalog, OnProgress: emitCheckProgress})
	if err != nil {
		return nil, err
	}
	results := make([]checkreport.CheckResult, 0, len(evaluated))
	for _, e := range evaluated {
		witnesses, truncated := checkreport.Witnesses(e.Tuples, e.Expect)
		results = append(results, checkreport.CheckResult{Name: e.Name, Query: e.Query, Expect: e.Expect, Severity: e.Severity, Message: e.Message, Scope: e.Scope, Outcome: e.Outcome, Passed: e.Passed, Count: e.Count, Evidence: e.Evidence, Diagnostics: e.Diagnostics, Witnesses: witnesses, WitnessesTruncated: truncated})
	}
	return results, nil
}

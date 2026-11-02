package operation

import (
	"context"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/importer"
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
// catalog) and returns the per-check verdicts. Rules are loaded from the
// workspace's rule paths for the model (including its rules/ subdir).
func runChecks(evalCtx context.Context, cat *runCatalog, ws Workspace, out Output, m *systemmodel.SystemModel, modelDir string, ctx checkContext) ([]checkreport.CheckResult, error) {
	if len(m.Checks) == 0 {
		return nil, nil
	}
	db, err := cat.required()
	if err != nil {
		return nil, err
	}
	reg, err := ws.Projections()
	if err != nil {
		return nil, err
	}
	evaluated, err := checks.Evaluate(evalCtx, checks.Request{Catalog: db, Registry: reg, RulePaths: ws.ModelRulePaths(modelDir), Load: importer.DecodeEntry, Checks: m.Checks, RunID: ctx.runID, CurrentSnapshot: ctx.currentSnapshot, FromCatalog: ctx.fromCatalog, OnProgress: func(event checks.Progress) { out.progress(event.Phase, event.Check, event.State) }})
	if err != nil {
		return nil, err
	}
	results := make([]checkreport.CheckResult, 0, len(evaluated))
	for _, e := range evaluated {
		witnesses, truncated := checkreport.Witnesses(e.Tuples, e.Expect)
		findings, findingsTruncated := checkreport.MatchFindings(e.Tuples, e.Expect, e.Outcome, witnesses)
		results = append(results, checkreport.CheckResult{Name: e.Name, Query: e.Query, Expect: e.Expect, Severity: e.Severity, Message: e.Message, Scope: e.Scope, Outcome: e.Outcome, Passed: e.Passed, Count: e.Count, Evidence: e.Evidence, Diagnostics: e.Diagnostics, Witnesses: witnesses, WitnessesTruncated: truncated, Findings: findings, FindingsTruncated: findingsTruncated})
	}
	return results, nil
}

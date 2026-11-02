package operation

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/ownership"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/waiver"
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
	// waivers matches each finding against the model's waivers; nil waives
	// nothing.
	waivers *waiver.Matcher
}

// runChecks evaluates each of the model's checks (a Datalog relation over the
// catalog) and returns the per-check verdicts, each finding matched against
// the model's waivers. Rules are loaded from the workspace's rule paths for
// the model (including its rules/ subdir).
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
		result := checkreport.CheckResult{Name: e.Name, Query: e.Query, Expect: e.Expect, Severity: e.Severity, Message: e.Message, Scope: e.Scope, Outcome: e.Outcome, Passed: e.Passed, Count: e.Count, Evidence: e.Evidence, Diagnostics: e.Diagnostics}
		if err := result.SetFindings(e.Tuples, findingMatcher(ctx.waivers, e.Name)); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	owners, err := ownership.Load(db)
	if err != nil {
		return nil, fmt.Errorf("load resource owners: %w", err)
	}
	checkreport.AttributeOwners(results, owners.OwnerOf)
	return results, nil
}

package repair

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/policyfacts"
)

// policyResults re-evaluates compliance policies over stored current
// resources (`pudl policy evaluate`). A result is stale when its policy digest
// differs from the loaded policy's; results of entries that were deleted or
// re-identified are closed, and resources never evaluated are evaluated. A
// policy that fails to load is reported and its results are left unchanged.
var policyResults = Repair{
	Name:      "policy-results",
	Condition: "policy results that are stale, orphaned or missing for the loaded policies",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		if env.Inferrer == nil {
			return Result{Status: StatusSkipped, Message: "no schemas loaded"}, nil
		}
		policies := policyfacts.New(env.Inferrer)
		reasons, err := policies.Stale(env.DB)
		if err != nil {
			return Result{}, err
		}
		result := Result{Pending: len(reasons), Message: "Policy results are current", Errors: loadErrors(policies)}
		if len(reasons) > 0 {
			result.Message = strings.Join(reasons, "; ")
		}
		return result, nil
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		if env.Inferrer == nil {
			return Result{Status: StatusSkipped, Message: "no schemas loaded"}, nil
		}
		policies := policyfacts.New(env.Inferrer)
		report, err := policies.Refresh(ctx, env.DB, importer.DecodeEntry, policyfacts.Options{DryRun: dryRun})
		if err != nil {
			return Result{}, err
		}
		result := Result{Pending: report.Updated + report.Closed + report.Orphaned, Details: report}
		if !dryRun {
			result.Changed = result.Pending
		}
		result.Message = fmt.Sprintf("%d resources evaluated: %d results updated, %d closed, %d orphaned results closed",
			report.Resources, report.Updated, report.Closed, report.Orphaned)
		result.Errors = append(loadErrors(policies), report.Errors...)
		result.Failed = len(report.Errors)
		return result, nil
	},
}

func loadErrors(policies *policyfacts.Evaluator) []string {
	var out []string
	for _, le := range policies.LoadErrors() {
		out = append(out, fmt.Sprintf("%v (existing results unchanged)", le))
	}
	return out
}

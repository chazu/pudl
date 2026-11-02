package repair

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/projection"
)

// projections brings schema-projected facts in line with the catalog and the
// current `_pudl.facts` specs, as corrections (formerly `pudl facts
// reproject`). It closes facts of entries that were deleted, re-identified or
// reassigned, recomputes resources projected with an older facts block, and
// projects resources never projected. A schema whose facts block is invalid is
// reported and its existing facts are left unchanged.
var projections = Repair{
	Name:      "projections",
	Condition: "projected facts that no longer match the catalog or the current facts specs",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		reasons, err := projection.Stale(env.DB, env.Projections)
		if err != nil {
			return Result{}, err
		}
		if len(reasons) == 0 {
			return Result{Message: "Projected facts are current"}, nil
		}
		return Result{Pending: len(reasons), Message: strings.Join(reasons, "; ")}, nil
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		report, err := projection.Sync(ctx, env.DB, env.Projections, importer.DecodeEntry, dryRun)
		if err != nil {
			return Result{}, err
		}
		result := Result{Pending: report.Bootstrapped + report.Reprojected + report.Orphaned, Details: report}
		if !dryRun {
			result.Changed = result.Pending
		}
		result.Message = fmt.Sprintf("%d bootstrapped, %d reprojected, %d orphaned sources closed",
			report.Bootstrapped, report.Reprojected, report.Orphaned)
		for schema, reason := range report.Broken {
			result.Errors = append(result.Errors, fmt.Sprintf("projection disabled for %s (existing facts unchanged): %s", schema, reason))
		}
		result.Errors = append(result.Errors, report.Errors...)
		result.Failed = len(report.Errors)
		return result, nil
	},
}

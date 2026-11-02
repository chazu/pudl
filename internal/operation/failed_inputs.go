package operation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/observe"
	"github.com/chazu/pudl/internal/populate"
	"github.com/chazu/pudl/internal/projection"
)

// errFailedInputs is a model's outcome when its populate ingested the runs
// that succeeded but some failed: the observation is incomplete.
var errFailedInputs = errors.New("populate: one or more inputs failed; their resources are unknown")

// failedInputsLabelLimit bounds how many failed inputs the run error names;
// the report lists them all.
const failedInputsLabelLimit = 5

// failedInputsError is the run outcome for a populate with failed inputs, or
// nil when every run succeeded.
func failedInputsError(pr *checkreport.Populate) error {
	if pr == nil || len(pr.FailedInputs) == 0 {
		return nil
	}
	labels := make([]string, 0, failedInputsLabelLimit+1)
	for i, f := range pr.FailedInputs {
		if i == failedInputsLabelLimit {
			labels = append(labels, fmt.Sprintf("and %d more", len(pr.FailedInputs)-i))
			break
		}
		labels = append(labels, f.Label())
	}
	return fmt.Errorf("%w: %d of %d runs failed (%s)", errFailedInputs, len(pr.FailedInputs), pr.Runs, strings.Join(labels, "; "))
}

// allInputsFailed is the populate error when no run succeeded: nothing is
// ingested. One failed run reads as it always has.
func allInputsFailed(failed []populate.FailedInput) error {
	if len(failed) == 1 {
		return failed[0].Err()
	}
	return fmt.Errorf("populate: all %d runs failed, so nothing was ingested (the report lists them); first: %w", len(failed), failed[0].Err())
}

// failedInputResults records each failed input as an errored observe result,
// which makes the snapshot incomplete: it keeps the populate's records but
// cannot establish what the failed inputs would have shown, so their
// resources' absence is never concluded and complete scopes are not closed.
func failedInputResults(target string, failed []populate.FailedInput) []observe.ObserveResult {
	out := make([]observe.ObserveResult, 0, len(failed))
	for _, f := range failed {
		out = append(out, observe.ObserveResult{
			Target: fmt.Sprintf("%s[%d]", target, f.Run),
			Error:  fmt.Sprintf("%s: exit %d: %s", f.Label(), f.ExitStatus, f.Error),
		})
	}
	return out
}

// markFailedInputChecks makes unknown every check that read the catalog
// directly (run or latest-known scope) while this run's populate had failed
// inputs: such a check cannot see the failed inputs' resources, so it can
// establish neither a pass nor a complete set of findings. Checks that select
// evidence are left alone; selecting the incomplete snapshot already makes
// them unknown.
func markFailedInputChecks(results []checkreport.CheckResult, pr *checkreport.Populate) {
	if pr == nil || len(pr.FailedInputs) == 0 {
		return
	}
	diagnostic := projection.Diagnostic{
		Code:    "failed_inputs",
		Message: fmt.Sprintf("%d of %d populate runs failed; their resources are unknown", len(pr.FailedInputs), pr.Runs),
	}
	for i := range results {
		c := &results[i]
		if c.Scope == checks.ScopeSnapshots || c.Uncertain() {
			continue
		}
		c.Outcome, c.Passed = "unknown", false
		c.Diagnostics = append(c.Diagnostics, diagnostic)
	}
}

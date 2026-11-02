package operation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/populate"
)

// TestFailedInputsLeaveFindingsUnknown: a run whose populate had a failed
// input reaches no verdict on its latest-known checks, so the findings an
// earlier report recorded are unknown, not resolved.
func TestFailedInputsLeaveFindingsUnknown(t *testing.T) {
	earlier := checkreport.CheckResult{Name: "c", Query: "q", Expect: "empty", Severity: "fail", Scope: checks.ScopeLatestKnown, Outcome: "fail",
		Findings: []checkreport.Finding{{Key: "k", State: checkreport.FindingNew, Arguments: map[string]any{"name": "a"}}}}
	now := checkreport.CheckResult{Name: "c", Query: "q", Expect: "empty", Severity: "fail", Scope: checks.ScopeLatestKnown, Outcome: "pass", Passed: true}
	results := []checkreport.CheckResult{now}
	markFailedInputChecks(results, &checkreport.Populate{Runs: 2, FailedInputs: []populate.FailedInput{{Run: 1}}})

	at := time.Now()
	findings, changes := checkreport.Classify(
		checkreport.NewObservation("run_1", "observe-only", at, results[0], false),
		[]checkreport.Observation{checkreport.NewObservation("run_0", "observe-only", at.Add(-time.Hour), earlier, true)})
	assert.Equal(t, 0, changes.Resolved)
	assert.Equal(t, 1, changes.Unknown)
	assert.Equal(t, checkreport.FindingUnknown, findings[0].State)
}

// TestReportHasNewFailures: only a new, unwaived finding of a failing
// fail-severity check is a new failure for --fail-on new.
func TestReportHasNewFailures(t *testing.T) {
	check := func(severity, outcome string, findings ...checkreport.Finding) *checkreport.ModelReport {
		c := checkreport.CheckResult{Name: "c", Severity: severity, Outcome: outcome, Findings: findings, Changes: &checkreport.Changes{}}
		return &checkreport.ModelReport{Checks: []checkreport.CheckResult{c}}
	}
	newFinding := checkreport.Finding{Key: "k", State: checkreport.FindingNew}
	waivedNew := checkreport.Finding{Key: "w", State: checkreport.FindingNew, Waived: true}
	persisting := checkreport.Finding{Key: "p", State: checkreport.FindingPersisting}

	assert.True(t, reportHasNewFailures(check("fail", "fail", newFinding, persisting)))
	assert.False(t, reportHasNewFailures(check("fail", "fail", persisting)))
	assert.False(t, reportHasNewFailures(check("fail", "fail", waivedNew, persisting)), "a waived new finding is not a new failure")
	assert.False(t, reportHasNewFailures(check("fail", checkreport.OutcomeWaived, waivedNew)))
	assert.False(t, reportHasNewFailures(check("warn", "fail", newFinding)), "warn-severity checks never fail a check")
	unclassified := check("fail", "fail")
	unclassified.Checks[0].Changes = nil
	assert.True(t, reportHasNewFailures(unclassified), "an unclassified failure may be new")
}

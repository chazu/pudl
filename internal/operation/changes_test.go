package operation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/checkreport"
)

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

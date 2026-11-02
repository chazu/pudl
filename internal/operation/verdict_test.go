package operation

import (
	"testing"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/stretchr/testify/assert"
)

func TestRunVerdict_FailSeverityCheckDemotesClean(t *testing.T) {
	drifted := &checkreport.ModelReport{
		Drift:  &acute.ModelDriftResult{Clean: true, Verified: true},
		Checks: []checkreport.CheckResult{{Name: "orphans", Severity: "fail", Passed: false}},
	}
	assert.Equal(t, "drifted", runVerdict(drifted))
}

func TestRunVerdict_NonFailSeverityCheckDoesNotDemote(t *testing.T) {
	report := &checkreport.ModelReport{
		Drift: &acute.ModelDriftResult{Clean: true, Verified: true},
		Checks: []checkreport.CheckResult{
			{Name: "warned", Severity: "warn", Passed: false},
			{Name: "noted", Severity: "info", Passed: false},
		},
	}
	assert.Equal(t, "clean", runVerdict(report))
}

func TestRunVerdict_DemotionOnlyAppliesToClean(t *testing.T) {
	failing := []checkreport.CheckResult{{Name: "orphans", Severity: "fail", Passed: false}}

	drifted := &checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: false, Verified: true}, Checks: failing}
	assert.Equal(t, "drifted", runVerdict(drifted))

	uncertain := &checkreport.ModelReport{Drift: &acute.ModelDriftResult{Uncertain: true}, Checks: failing}
	assert.Equal(t, "unknown", runVerdict(uncertain))

	// A replay writes no model status, and a check does not change that it
	// observed nothing live.
	replay := &checkreport.ModelReport{
		Drift:  &acute.ModelDriftResult{Clean: true, Verified: false},
		Checks: failing,
	}
	assert.Equal(t, "", runVerdict(replay))
}

func TestRunVerdict_PassingChecksLeaveCleanIntact(t *testing.T) {
	report := &checkreport.ModelReport{
		Drift:  &acute.ModelDriftResult{Clean: true, Verified: true},
		Checks: []checkreport.CheckResult{{Name: "orphans", Severity: "fail", Passed: true}},
	}
	assert.Equal(t, "clean", runVerdict(report))
}

func TestFailedFailSeverityNames(t *testing.T) {
	names := failedFailSeverityNames([]checkreport.CheckResult{
		{Name: "a", Severity: "fail", Passed: false},
		{Name: "b", Severity: "warn", Passed: false},
		{Name: "c", Severity: "fail", Passed: true},
		{Name: "d", Severity: "fail", Passed: false},
	})
	assert.Equal(t, []string{"a", "d"}, names)
}

func TestRunFinishState_NotesAccumulate(t *testing.T) {
	state := &runFinishState{}
	state.addNote("")
	assert.Equal(t, "", state.note)
	state.addNote("demoted by check")
	state.addNote("run error")
	assert.Equal(t, "demoted by check; run error", state.note)
}

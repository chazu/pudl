package cmd

import (
	"testing"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/stretchr/testify/assert"
)

func TestHeadExposesRunID(t *testing.T) {
	rules := []datalog.Rule{
		{
			Name: "scoped",
			Head: datalog.Atom{Rel: "scoped_check", Args: map[string]datalog.Term{
				"run_id": datalog.Var("$R"), "target": datalog.Var("$T"),
			}},
		},
		{
			Name: "global",
			Head: datalog.Atom{Rel: "global_check", Args: map[string]datalog.Term{
				"target": datalog.Var("$T"),
			}},
		},
		{
			Name: "ground",
			Head: datalog.Atom{Rel: "ground_check", Args: map[string]datalog.Term{
				"run_id": datalog.Val("literal"),
			}},
		},
	}

	assert.True(t, headExposesRunID(rules, "scoped_check"))
	assert.False(t, headExposesRunID(rules, "global_check"))
	assert.False(t, headExposesRunID(rules, "ground_check"),
		"a ground head arg is not a binding point; constraining it would be a no-op or a contradiction")
	assert.False(t, headExposesRunID(rules, "relation_with_no_rule"))
}

func TestRunVerdict_FailSeverityCheckDemotesClean(t *testing.T) {
	drifted := &RunReport{
		Drift:  &ModelDriftResult{Clean: true, Verified: true},
		Checks: []CheckResult{{Name: "orphans", Severity: "fail", Passed: false}},
	}
	assert.Equal(t, "drifted", runVerdict(drifted))
}

func TestRunVerdict_NonFailSeverityCheckDoesNotDemote(t *testing.T) {
	report := &RunReport{
		Drift: &ModelDriftResult{Clean: true, Verified: true},
		Checks: []CheckResult{
			{Name: "warned", Severity: "warn", Passed: false},
			{Name: "noted", Severity: "info", Passed: false},
		},
	}
	assert.Equal(t, "clean", runVerdict(report))
}

func TestRunVerdict_DemotionOnlyAppliesToClean(t *testing.T) {
	failing := []CheckResult{{Name: "orphans", Severity: "fail", Passed: false}}

	drifted := &RunReport{Drift: &ModelDriftResult{Clean: false, Verified: true}, Checks: failing}
	assert.Equal(t, "drifted", runVerdict(drifted))

	uncertain := &RunReport{Drift: &ModelDriftResult{Uncertain: true}, Checks: failing}
	assert.Equal(t, "unknown", runVerdict(uncertain))

	// A replay writes no model status, and a check does not change that it
	// observed nothing live.
	replay := &RunReport{
		Drift:  &ModelDriftResult{Clean: true, Verified: false},
		Checks: failing,
	}
	assert.Equal(t, "", runVerdict(replay))
}

func TestRunVerdict_PassingChecksLeaveCleanIntact(t *testing.T) {
	report := &RunReport{
		Drift:  &ModelDriftResult{Clean: true, Verified: true},
		Checks: []CheckResult{{Name: "orphans", Severity: "fail", Passed: true}},
	}
	assert.Equal(t, "clean", runVerdict(report))
}

func TestFailedFailSeverityNames(t *testing.T) {
	names := failedFailSeverityNames([]CheckResult{
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

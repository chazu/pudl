package cmd

import (
	"testing"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkScopeFixture(t *testing.T, only ...string) (*systemmodel.SystemModel, *acute.TupleScope) {
	t.Helper()
	model := &systemmodel.SystemModel{
		Name: "m",
		Desired: []map[string]any{
			{"name": "app", "kind": "Deployment"},
			{"name": "db", "kind": "StatefulSet"},
		},
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
	}
	effective, err := acute.ScopeModelForRun(model, only)
	require.NoError(t, err)
	return model, acute.NewTupleScope(model, effective)
}

func tuple(values ...string) datalog.Tuple {
	args := map[string]interface{}{}
	for i, v := range values {
		args[string(rune('a'+i))] = v
	}
	return datalog.Tuple{Relation: "r", Args: args}
}

func TestPartitionCheckTuples_UnscopedRunPartitionsNothing(t *testing.T) {
	_, scope := checkScopeFixture(t)
	gating, advisory := partitionCheckTuples([]datalog.Tuple{tuple("app"), tuple("db")}, "empty", scope)
	assert.Equal(t, 2, gating)
	assert.Equal(t, 0, advisory)
}

func TestPartitionCheckTuples_EmptyCheckExcusesOutOfScopeRows(t *testing.T) {
	_, scope := checkScopeFixture(t, "app")
	gating, advisory := partitionCheckTuples([]datalog.Tuple{tuple("db"), tuple("db")}, "empty", scope)
	assert.Equal(t, 0, gating, "matches on the excluded resource do not gate")
	assert.Equal(t, 2, advisory)
	assert.True(t, checkPasses("empty", gating))
}

func TestPartitionCheckTuples_EmptyCheckKeepsInScopeRows(t *testing.T) {
	_, scope := checkScopeFixture(t, "app")
	gating, advisory := partitionCheckTuples([]datalog.Tuple{tuple("app"), tuple("db")}, "empty", scope)
	assert.Equal(t, 1, gating)
	assert.Equal(t, 1, advisory)
	assert.False(t, checkPasses("empty", gating))
}

func TestPartitionCheckTuples_NonemptyCheckNeverPartitions(t *testing.T) {
	// A nonempty check counts evidence, not violations. Dropping an out-of-scope
	// row could only manufacture a failure nothing in scope can fix.
	_, scope := checkScopeFixture(t, "app")
	gating, advisory := partitionCheckTuples([]datalog.Tuple{tuple("db")}, "nonempty", scope)
	assert.Equal(t, 1, gating)
	assert.Equal(t, 0, advisory)
	assert.True(t, checkPasses("nonempty", gating))
}

func TestPartitionCheckTuples_UnresolvableRowGates(t *testing.T) {
	_, scope := checkScopeFixture(t, "app")
	gating, advisory := partitionCheckTuples([]datalog.Tuple{tuple("mystery-resource")}, "empty", scope)
	assert.Equal(t, 1, gating, "an unclassifiable row is fail-safe: it gates")
	assert.Equal(t, 0, advisory)
}

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
	converged := &RunReport{
		Converge: &ConvergeReport{Outcome: string(outcomeClean)},
		Checks:   []CheckResult{{Name: "orphans", Severity: "fail", Passed: false}},
	}
	assert.Equal(t, "drifted", runVerdict(converged, runFlags{converge: true}))

	drifted := &RunReport{
		Drift:  &ModelDriftResult{Clean: true, Verified: true},
		Checks: []CheckResult{{Name: "orphans", Severity: "fail", Passed: false}},
	}
	assert.Equal(t, "drifted", runVerdict(drifted, runFlags{}),
		"the observe-only arm has the same hole as the converge arm")
}

func TestRunVerdict_NonFailSeverityCheckDoesNotDemote(t *testing.T) {
	report := &RunReport{
		Converge: &ConvergeReport{Outcome: string(outcomeClean)},
		Checks: []CheckResult{
			{Name: "warned", Severity: "warn", Passed: false},
			{Name: "noted", Severity: "info", Passed: false},
		},
	}
	assert.Equal(t, "clean", runVerdict(report, runFlags{converge: true}))
}

func TestRunVerdict_DemotionOnlyAppliesToClean(t *testing.T) {
	failing := []CheckResult{{Name: "orphans", Severity: "fail", Passed: false}}

	// `unknown` means the run could not prove the state at all; a check over a
	// catalog possibly missing this run's receipt cannot upgrade that.
	needsVerification := &RunReport{
		Converge: &ConvergeReport{Outcome: string(outcomeClean), NeedsVerification: true},
		Checks:   failing,
	}
	assert.Equal(t, "unknown", runVerdict(needsVerification, runFlags{converge: true}))

	failed := &RunReport{
		Converge: &ConvergeReport{Outcome: "failed (cap_exhausted)"},
		Checks:   failing,
	}
	assert.Equal(t, "failed", runVerdict(failed, runFlags{converge: true}))

	// A replay writes no model status, and a check does not change that it
	// observed nothing live.
	replay := &RunReport{
		Drift:  &ModelDriftResult{Clean: true, Verified: false},
		Checks: failing,
	}
	assert.Equal(t, "", runVerdict(replay, runFlags{fromCatalog: true}))

	dry := &RunReport{
		Converge: &ConvergeReport{Outcome: string(outcomeClean)},
		Checks:   failing,
	}
	assert.Equal(t, "", runVerdict(dry, runFlags{converge: true, dryRun: true}))
}

func TestRunVerdict_PassingChecksLeaveCleanIntact(t *testing.T) {
	report := &RunReport{
		Converge: &ConvergeReport{Outcome: string(outcomeClean)},
		Checks:   []CheckResult{{Name: "orphans", Severity: "fail", Passed: true}},
	}
	assert.Equal(t, "clean", runVerdict(report, runFlags{converge: true}))
}

func TestPrintChecks_AdvisoryDoesNotGate(t *testing.T) {
	// A check whose only matches were out of scope passes, so the exit code stays
	// zero — and the rendering must not claim FAIL alongside it.
	assert.False(t, printChecks([]CheckResult{
		{Name: "orphans", Severity: "fail", Passed: true, AdvisoryCount: 3},
	}))
	// A check with both gating and advisory matches still gates.
	assert.True(t, printChecks([]CheckResult{
		{Name: "orphans", Severity: "fail", Passed: false, Count: 1, AdvisoryCount: 2},
	}))
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
	state.addNote("scoped run")
	assert.Equal(t, "demoted by check; scoped run", state.note)
}

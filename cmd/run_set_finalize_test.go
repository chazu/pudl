package cmd

import (
	"testing"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A run-set converge member must conclude exactly as a standalone converge
// run does: checks run, the verdict lands on the model row, and a verified
// clean promotes the model's converging resources. The set path used to call
// the converge loop directly and skip all three.

const setMemberRunID = "run_set_member"

// setConvergeFixture stages one approved, mutation-required member whose model
// declares a fail-severity check, plus a resource a prior apply left
// `converging` for that model.
func setConvergeFixture(t *testing.T) (*database.CatalogDB, *preparedMutationMember, *scriptedMu) {
	t.Helper()
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })

	cat, muRoot := acceptanceFixture(t)
	db, err := cat.required()
	require.NoError(t, err)

	model := convergentModel()
	model.Checks = []systemmodel.Check{{
		Name: "no-failures", Query: "failed_anywhere", Expect: "empty", Severity: "fail",
	}}
	require.NoError(t, recordModelInstance(cat, model, setMemberRunID))
	require.NoError(t, db.StartRun(setMemberRunID, model.Name, "converge"))

	target, entryType, tags := "Deployment/nginx", "manifest-action", `{"exit_code":0,"model":"m"}`
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: "pending-apply", StoredPath: "p.json", MetadataPath: "p.meta",
		ImportTimestamp: time.Now(), Format: "json", Origin: "t",
		Schema: "pudl/core.#Item", Target: &target, EntryType: &entryType, Tags: &tags,
	}))
	require.NoError(t, db.UpdateStatus(target, "converging"))

	member := &preparedMutationMember{
		model: model, runID: setMemberRunID, muRoot: muRoot, modelDir: checksRulesDir(t), required: true,
		report: &RunReport{ReportVersion: 1, RunSetID: "set_x", RunID: setMemberRunID, Model: model.Name, Mode: "converge"},
	}
	mu := &scriptedMu{
		observes:  [][]byte{[]byte(driftedObserve), []byte(cleanObserve)},
		manifests: [][]byte{[]byte(`{"actions":[]}`)},
	}
	return db, member, mu
}

func executeSetMember(t *testing.T, db *database.CatalogDB, mu muRunner, member *preparedMutationMember) (*acute.RunSetReport, error) {
	t.Helper()
	report := &acute.RunSetReport{
		ReportVersion: 1, RunSetID: "set_x", Mode: "converge", Status: database.RunStatusRunning,
		Ordered: []string{member.model.Name},
		Members: []acute.RunSetMemberReport{{Model: member.model.Name, RunID: member.runID, Result: database.RunStatusRunning}},
	}
	plan := &acute.RunSetMutationPlan{
		RunSetID: "set_x", Ordered: []string{member.model.Name},
		Options: acute.RunSetMutationOptions{MaxIterations: 3, MaxApplies: 10},
	}
	err := executePreparedMutationPlan(db, mu, report, plan, map[string]*preparedMutationMember{member.model.Name: member})
	return report, err
}

func targetStatuses(t *testing.T, db *database.CatalogDB) map[string]string {
	t.Helper()
	statuses, err := db.GetTargetStatuses()
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range statuses {
		got[s.Target] = s.Status
	}
	return got
}

func TestAcceptance_SetConvergeMemberIsFinalizedLikeStandaloneRun(t *testing.T) {
	db, member, mu := setConvergeFixture(t)

	report, err := executeSetMember(t, db, mu, member)
	require.NoError(t, err)
	assert.Equal(t, database.RunStatusSucceeded, report.Status)

	require.Len(t, member.report.Checks, 1, "the member's checks were evaluated")
	assert.True(t, member.report.Checks[0].Passed)
	assert.Equal(t, database.RunStatusSucceeded, member.report.CompletionStatus)

	statuses := targetStatuses(t, db)
	assert.Equal(t, "clean", statuses[modelTargetKey("m")], "the verdict is recorded on the model row")
	assert.Equal(t, "clean", statuses["Deployment/nginx"], "converging resources are promoted after a verified clean")

	run, err := db.GetRun(setMemberRunID)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "clean", run.Verdict)
	assert.Equal(t, string(outcomeClean), run.Outcome)
}

func TestAcceptance_SetConvergeMemberFailingCheckFailsMemberAndDemotesVerdict(t *testing.T) {
	db, member, mu := setConvergeFixture(t)
	seedFailedEntry(t, db, "broken", "some-resource", "run_previous")

	report, err := executeSetMember(t, db, mu, member)
	require.ErrorContains(t, err, "run set set_x failed")
	assert.Equal(t, database.RunStatusFailed, report.Status)
	assert.Equal(t, database.RunStatusFailed, report.Members[0].Result)

	require.Len(t, member.report.Checks, 1)
	assert.False(t, member.report.Checks[0].Passed)

	statuses := targetStatuses(t, db)
	assert.Equal(t, "drifted", statuses[modelTargetKey("m")], "a failed fail-severity check demotes clean to drifted")

	run, err := db.GetRun(setMemberRunID)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "drifted", run.Verdict)
	assert.Contains(t, run.Note, "no-failures")
	assert.Contains(t, run.Note, errFailSeverityChecks.Error())
}

package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runsTestDB(t *testing.T) *CatalogDB {
	t.Helper()
	dir, err := os.MkdirTemp("", "pudl-runs-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	db, err := NewCatalogDB(dir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStartRunThenFinishRunIsTerminal(t *testing.T) {
	db := runsTestDB(t)

	require.NoError(t, db.StartRun("run_a", "model-a", "converge"))

	started, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, started)
	assert.False(t, started.Finished(), "a started run is not yet terminal")
	assert.Equal(t, "model-a", started.Model)
	assert.Equal(t, "converge", started.Mode)

	require.NoError(t, db.FinishRun("run_a", RunConclusion{CompletionStatus: RunStatusFailed, Verdict: "unknown", Outcome: "failed (cap_exhausted)", NeedsVerification: true, Note: "receipt lost"}))

	finished, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.True(t, finished.Finished())
	assert.Equal(t, "unknown", finished.Verdict)
	assert.Equal(t, "failed (cap_exhausted)", finished.Outcome)
	assert.True(t, finished.NeedsVerification)
	assert.Equal(t, "receipt lost", finished.Note)
	assert.Equal(t, RunStatusFailed, finished.CompletionStatus)
}

func TestPrepareRunMutationReopensOnlySuccessfulPreflight(t *testing.T) {
	db := runsTestDB(t)
	require.NoError(t, db.StartRun("run_ok", "model-a", "observe-only"))
	require.NoError(t, db.FinishRun("run_ok", RunConclusion{CompletionStatus: RunStatusSucceeded}))
	require.NoError(t, db.PrepareRunMutation("run_ok"))

	record, err := db.GetRun("run_ok")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "converge", record.Mode)
	assert.Equal(t, RunStatusRunning, record.CompletionStatus)
	assert.False(t, record.Finished())

	require.NoError(t, db.StartRun("run_failed", "model-a", "observe-only"))
	require.NoError(t, db.FinishRun("run_failed", RunConclusion{CompletionStatus: RunStatusFailed}))
	assert.Error(t, db.PrepareRunMutation("run_failed"))
}

func TestMutationIntentIsDurableUntilReceiptCommits(t *testing.T) {
	db := runsTestDB(t)
	require.NoError(t, db.StartRun("run_attempt", "model-a", "converge"))
	require.NoError(t, db.BeginRunMutationAttempt("run_attempt"))

	crashed, err := db.GetRun("run_attempt")
	require.NoError(t, err)
	require.NotNil(t, crashed)
	assert.False(t, crashed.Finished())
	assert.Equal(t, "unknown", crashed.Verdict)
	assert.Equal(t, "needs-verification", crashed.Outcome)
	assert.True(t, crashed.NeedsVerification)
	assert.Contains(t, crashed.Note, "receipt not recorded")

	require.NoError(t, db.RecordApply("run_attempt"))
	require.NoError(t, db.CompleteRunMutationReceipt("run_attempt"))
	receipted, err := db.GetRun("run_attempt")
	require.NoError(t, err)
	require.NotNil(t, receipted)
	assert.False(t, receipted.NeedsVerification)
	assert.Empty(t, receipted.Outcome)
	assert.Empty(t, receipted.Note)
	assert.Equal(t, 1, receipted.Applies)
	assert.Error(t, db.CompleteRunMutationReceipt("run_attempt"), "one receipt cannot clear twice")
}

// The point of the table: a run that never finished stays discoverable, so the
// status its model carries can be recognised as predating that run.
func TestUnfinishedRunsFindsOnlyRunsThatNeverFinished(t *testing.T) {
	db := runsTestDB(t)

	require.NoError(t, db.StartRun("run_done", "model-a", "converge"))
	require.NoError(t, db.FinishRun("run_done", RunConclusion{CompletionStatus: RunStatusSucceeded, Verdict: "clean", Outcome: "clean"}))
	require.NoError(t, db.StartRun("run_crashed", "model-a", "converge"))
	require.NoError(t, db.StartRun("run_other", "model-b", "observe-only"))

	forModel, err := db.UnfinishedRuns("model-a")
	require.NoError(t, err)
	require.Len(t, forModel, 1)
	assert.Equal(t, "run_crashed", forModel[0].RunID)

	all, err := db.UnfinishedRuns("")
	require.NoError(t, err)
	assert.Len(t, all, 2, "both models' unfinished runs")
}

// A run that concluded without writing a status is still terminal. That has to be
// distinguishable from a run that died, or the crash signal is lost.
func TestFinishRunWithEmptyVerdictIsStillFinished(t *testing.T) {
	db := runsTestDB(t)

	require.NoError(t, db.StartRun("run_a", "model-a", "observe-only"))
	require.NoError(t, db.FinishRun("run_a", RunConclusion{CompletionStatus: RunStatusFailed, Note: "populate failed"}))

	record, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.True(t, record.Finished())
	assert.Empty(t, record.Verdict)
	assert.Equal(t, "populate failed", record.Note)

	unfinished, err := db.UnfinishedRuns("model-a")
	require.NoError(t, err)
	assert.Empty(t, unfinished)
}

func TestStartRunIsIdempotent(t *testing.T) {
	db := runsTestDB(t)

	require.NoError(t, db.StartRun("run_a", "model-a", "observe-only"))
	require.NoError(t, db.StartRun("run_a", "model-a", "converge"))

	record, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "converge", record.Mode)
}

func TestFinishRunOnUnknownRunErrors(t *testing.T) {
	db := runsTestDB(t)

	err := db.FinishRun("nope", RunConclusion{CompletionStatus: RunStatusSucceeded, Verdict: "clean", Outcome: "clean"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such run")
}

func TestGetRunMissingReturnsNil(t *testing.T) {
	db := runsTestDB(t)

	record, err := db.GetRun("nope")

	require.NoError(t, err)
	assert.Nil(t, record)
}

// The migration must be safe on an existing database, like every other one.
func TestEnsureRunsTableIsIdempotent(t *testing.T) {
	db := runsTestDB(t)

	require.NoError(t, db.ensureRunsTable())
	require.NoError(t, db.ensureRunsTable())

	require.NoError(t, db.StartRun("run_a", "model-a", "converge"))
	record, err := db.GetRun("run_a")
	require.NoError(t, err)
	assert.NotNil(t, record)
}

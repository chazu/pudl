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

	require.NoError(t, db.FinishRun("run_a", "unknown", "failed (cap_exhausted)", true, "receipt lost"))

	finished, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.True(t, finished.Finished())
	assert.Equal(t, "unknown", finished.Verdict)
	assert.Equal(t, "failed (cap_exhausted)", finished.Outcome)
	assert.True(t, finished.NeedsVerification)
	assert.Equal(t, "receipt lost", finished.Note)
}

// The point of the table: a run that never finished stays discoverable, so the
// status its model carries can be recognised as predating that run.
func TestUnfinishedRunsFindsOnlyRunsThatNeverFinished(t *testing.T) {
	db := runsTestDB(t)

	require.NoError(t, db.StartRun("run_done", "model-a", "converge"))
	require.NoError(t, db.FinishRun("run_done", "clean", "clean", false, ""))
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
	require.NoError(t, db.FinishRun("run_a", "", "", false, "populate failed"))

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

	err := db.FinishRun("nope", "clean", "clean", false, "")

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

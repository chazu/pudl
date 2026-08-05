package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applyRun records one finished run of a model with the given applies.
func applyRun(t *testing.T, db *CatalogDB, runID, model, verdict string, applies int, scoped bool) {
	t.Helper()
	require.NoError(t, db.StartRun(runID, model, "converge"))
	for i := 0; i < applies; i++ {
		require.NoError(t, db.RecordApply(runID))
	}
	require.NoError(t, db.FinishRun(runID, RunConclusion{CompletionStatus: RunStatusSucceeded, Verdict: verdict, Scoped: scoped}))
}

func TestAppliesSinceLastClean_NoHistory(t *testing.T) {
	db := runsTestDB(t)
	spent, err := db.AppliesSinceLastClean("never-run")
	require.NoError(t, err)
	assert.Equal(t, 0, spent)
}

func TestAppliesSinceLastClean_SumsUntilTheLastCleanRun(t *testing.T) {
	db := runsTestDB(t)

	applyRun(t, db, "run_1", "m", "failed", 3, false)
	applyRun(t, db, "run_2", "m", "clean", 2, false)
	applyRun(t, db, "run_3", "m", "drifted", 4, false)
	applyRun(t, db, "run_4", "m", "failed", 1, false)

	spent, err := db.AppliesSinceLastClean("m")
	require.NoError(t, err)
	assert.Equal(t, 5, spent, "runs 3 and 4 only; the clean run 2 stops the walk")
}

func TestAppliesSinceLastClean_ScopedCleanDoesNotReset(t *testing.T) {
	// A scoped ∅ does not prove the whole model clean — the same reason
	// modelRowVerdict degrades it to `unknown`. Letting it reset the budget would
	// let a scheduler alternating a converging scope with an oscillating one
	// refill the budget every other run.
	db := runsTestDB(t)

	applyRun(t, db, "run_1", "m", "failed", 3, false)
	applyRun(t, db, "run_2", "m", "clean", 2, true) // scoped clean
	applyRun(t, db, "run_3", "m", "failed", 1, false)

	spent, err := db.AppliesSinceLastClean("m")
	require.NoError(t, err)
	assert.Equal(t, 6, spent, "the walk continues through a scoped clean")
}

func TestAppliesSinceLastClean_CountsUnfinishedRuns(t *testing.T) {
	// The crash-loop case: a run killed after applying never reaches FinishRun,
	// but its applies are already durable and must still be charged.
	db := runsTestDB(t)

	require.NoError(t, db.StartRun("run_crashed", "m", "converge"))
	require.NoError(t, db.RecordApply("run_crashed"))
	require.NoError(t, db.RecordApply("run_crashed"))
	// no FinishRun — the process died

	spent, err := db.AppliesSinceLastClean("m")
	require.NoError(t, err)
	assert.Equal(t, 2, spent)

	crashed, err := db.GetRun("run_crashed")
	require.NoError(t, err)
	require.NotNil(t, crashed)
	assert.False(t, crashed.Finished())
	assert.Equal(t, 2, crashed.Applies)
}

func TestAppliesSinceLastClean_IgnoresOtherModels(t *testing.T) {
	db := runsTestDB(t)

	applyRun(t, db, "run_a", "model-a", "failed", 5, false)
	applyRun(t, db, "run_b", "model-b", "failed", 2, false)

	spent, err := db.AppliesSinceLastClean("model-b")
	require.NoError(t, err)
	assert.Equal(t, 2, spent)
}

func TestRecordApply_UnknownRunIsAnError(t *testing.T) {
	db := runsTestDB(t)
	assert.Error(t, db.RecordApply("nope"))
	assert.Error(t, db.RecordApply(""))
}

func TestFinishRun_RecordsScope(t *testing.T) {
	db := runsTestDB(t)
	require.NoError(t, db.StartRun("run_a", "m", "converge"))
	require.NoError(t, db.FinishRun("run_a", RunConclusion{CompletionStatus: RunStatusSucceeded, Verdict: "clean", Scoped: true}))

	record, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.True(t, record.Scoped)
}

func TestRunsMigration_AddsBudgetColumnsToAnExistingTable(t *testing.T) {
	// A database created before the apply budget existed has the runs table but
	// neither column. Re-opening must add them without touching existing rows.
	db := runsTestDB(t)
	require.NoError(t, db.StartRun("run_a", "m", "converge"))

	_, err := db.DB().Exec("ALTER TABLE runs DROP COLUMN applies")
	require.NoError(t, err)
	_, err = db.DB().Exec("ALTER TABLE runs DROP COLUMN scoped")
	require.NoError(t, err)

	require.NoError(t, db.ensureRunsTable())

	record, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, 0, record.Applies)
	assert.False(t, record.Scoped)
	require.NoError(t, db.RecordApply("run_a"))
}

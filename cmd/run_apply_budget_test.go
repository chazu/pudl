package cmd

import (
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func budgetCatalog(t *testing.T) *runCatalog {
	t.Helper()
	cat := newRunCatalog(t.TempDir())
	t.Cleanup(func() { _ = cat.Close() })
	return cat
}

func TestResolveApplyBudget_NotAConvergeRun(t *testing.T) {
	cat := budgetCatalog(t)
	assert.Nil(t, resolveApplyBudget(cat, "m", runFlags{maxApplies: 20}, false),
		"an observe-only run applies nothing, so it has no budget to spend")
	assert.False(t, cat.opened, "and does not open the catalog to find that out")
}

func TestResolveApplyBudget_DryRunIsExempt(t *testing.T) {
	cat := budgetCatalog(t)
	assert.Nil(t, resolveApplyBudget(cat, "m", runFlags{converge: true, dryRun: true, maxApplies: 20}, false))
	assert.False(t, cat.opened, "a dry run must not so much as create the catalog file")
}

func TestResolveApplyBudget_ZeroDisablesIt(t *testing.T) {
	cat := budgetCatalog(t)
	assert.Nil(t, resolveApplyBudget(cat, "m", runFlags{converge: true, maxApplies: 0}, false),
		"--max-applies 0 is the escape hatch: unlimited, and auditable on the run row")
	assert.False(t, cat.opened)
}

func TestResolveApplyBudget_FreshModelGetsTheFullBudget(t *testing.T) {
	cat := budgetCatalog(t)
	budget := resolveApplyBudget(cat, "m", runFlags{converge: true, maxApplies: 20}, false)
	require.NotNil(t, budget)
	assert.Equal(t, 20, *budget)
}

func TestResolveApplyBudget_SubtractsSpentApplies(t *testing.T) {
	cat := budgetCatalog(t)
	db, err := cat.required()
	require.NoError(t, err)

	require.NoError(t, db.StartRun("run_1", "m", "converge"))
	for i := 0; i < 7; i++ {
		require.NoError(t, db.RecordApply("run_1"))
	}
	require.NoError(t, db.FinishRun("run_1", database.RunConclusion{Verdict: "failed"}))

	budget := resolveApplyBudget(cat, "m", runFlags{converge: true, maxApplies: 20}, false)
	require.NotNil(t, budget)
	assert.Equal(t, 13, *budget)
}

func TestResolveApplyBudget_ExhaustedFloorsAtZeroRatherThanGoingNegative(t *testing.T) {
	cat := budgetCatalog(t)
	db, err := cat.required()
	require.NoError(t, err)

	require.NoError(t, db.StartRun("run_1", "m", "converge"))
	for i := 0; i < 9; i++ {
		require.NoError(t, db.RecordApply("run_1"))
	}
	require.NoError(t, db.FinishRun("run_1", database.RunConclusion{Verdict: "failed"}))

	budget := resolveApplyBudget(cat, "m", runFlags{converge: true, maxApplies: 4}, false)
	require.NotNil(t, budget)
	assert.Equal(t, 0, *budget, "zero means observe-then-refuse, never a negative allowance")
}

func TestResolveApplyBudget_CleanRunRefillsIt(t *testing.T) {
	cat := budgetCatalog(t)
	db, err := cat.required()
	require.NoError(t, err)

	require.NoError(t, db.StartRun("run_1", "m", "converge"))
	for i := 0; i < 5; i++ {
		require.NoError(t, db.RecordApply("run_1"))
	}
	require.NoError(t, db.FinishRun("run_1", database.RunConclusion{Verdict: "failed"}))

	// The remediation loop: this run applied once and ended clean.
	require.NoError(t, db.StartRun("run_2", "m", "converge"))
	require.NoError(t, db.RecordApply("run_2"))
	require.NoError(t, db.FinishRun("run_2", database.RunConclusion{Verdict: "clean"}))

	budget := resolveApplyBudget(cat, "m", runFlags{converge: true, maxApplies: 20}, false)
	require.NotNil(t, budget)
	assert.Equal(t, 20, *budget,
		"a model that drifts and is fixed every run resets every run, and can run forever")
}

func TestResolveApplyBudget_UnreadableCatalogDoesNotRefuseToApply(t *testing.T) {
	// A catalog that cannot answer must not silently withhold applies: that
	// failure mode is worse than the one the budget prevents.
	cat := newRunCatalog("/proc/pudl-cannot-exist")
	t.Cleanup(func() { _ = cat.Close() })
	assert.Nil(t, resolveApplyBudget(cat, "m", runFlags{converge: true, maxApplies: 20}, false))
}

func TestValidateRunFlags_MaxApplies(t *testing.T) {
	assert.Error(t, validateRunFlags(runFlags{maxAppliesSet: true}),
		"--max-applies without --converge is meaningless")
	assert.Error(t, validateRunFlags(runFlags{converge: true, maxIters: 1, maxApplies: -1}))
	assert.NoError(t, validateRunFlags(runFlags{converge: true, maxIters: 1, maxApplies: 0}))
	assert.NoError(t, validateRunFlags(runFlags{converge: true, maxIters: 1, maxApplies: 20}))
}

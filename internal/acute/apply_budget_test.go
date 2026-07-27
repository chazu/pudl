package acute

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func budgetOf(n int) *int { return &n }

func dirty(n int) []Observation {
	observations := make([]Observation, n)
	for i := range observations {
		observations[i] = Observation{Clean: false}
	}
	return observations
}

func TestConverge_NilBudgetLeavesBehaviourUnchanged(t *testing.T) {
	// No catalog to consult means no durable constraint: the per-run cap alone
	// decides, exactly as before the budget existed.
	executor := &fakeExecutor{observations: append(dirty(3), Observation{Clean: true})}
	result, err := Converge(ConvergeRequest{
		Executor:      executor,
		MaxIterations: 5,
		ApplyBudget:   nil,
	})
	require.NoError(t, err)
	assert.Equal(t, OutcomeClean, result.Outcome)
	assert.Equal(t, 3, result.Iterations)
}

func TestConverge_BudgetBindsBeforeTheRunCap(t *testing.T) {
	// --max-iters 5 would allow five applies; the model has two left.
	executor := &fakeExecutor{observations: dirty(6)}
	result, err := Converge(ConvergeRequest{
		Executor:      executor,
		MaxIterations: 5,
		ApplyBudget:   budgetOf(2),
	})
	require.Error(t, err)
	assert.Equal(t, OutcomeBudgetExhausted, result.Outcome)
	assert.Equal(t, 2, result.Iterations)
	assert.Equal(t, 2, executor.applied, "the budget, not the cap, stopped the loop")
	assert.False(t, result.NeedsVerification)
}

func TestConverge_RunCapStillBindsWhenItIsTheSmaller(t *testing.T) {
	executor := &fakeExecutor{observations: dirty(6)}
	result, err := Converge(ConvergeRequest{
		Executor:      executor,
		MaxIterations: 2,
		ApplyBudget:   budgetOf(10),
	})
	require.Error(t, err)
	assert.Equal(t, OutcomeCapExhausted, result.Outcome)
	assert.Equal(t, 2, result.Iterations)
}

func TestConverge_ExhaustedBudgetStillObserves(t *testing.T) {
	// A budget of zero must not skip the observation: observing is how the budget
	// resets, so refusing to look would make exhaustion permanent.
	executor := &fakeExecutor{observations: dirty(1)}
	result, err := Converge(ConvergeRequest{
		Executor:      executor,
		MaxIterations: 5,
		ApplyBudget:   budgetOf(0),
	})
	require.Error(t, err)
	assert.Equal(t, OutcomeBudgetExhausted, result.Outcome)
	assert.Equal(t, 1, executor.observed, "it looked")
	assert.Equal(t, 0, executor.applied, "and applied nothing")
}

func TestConverge_ExhaustedBudgetOnAModelThatIsCleanEndsClean(t *testing.T) {
	// The reset path: the model converged since, so the budget never bites and
	// the clean verdict is what refills it.
	executor := &fakeExecutor{observations: []Observation{{Clean: true}}}
	result, err := Converge(ConvergeRequest{
		Executor:      executor,
		MaxIterations: 5,
		ApplyBudget:   budgetOf(0),
	})
	require.NoError(t, err)
	assert.Equal(t, OutcomeClean, result.Outcome)
	assert.Equal(t, 0, executor.applied)
}

func TestConverge_OnAppliedFiresPerSuccessfulApplyBeforeTheManifest(t *testing.T) {
	// Ordering is the point: the durable count must land before anything else in
	// the iteration can fail, or a crash-looping run spends applies invisibly.
	var order []string
	executor := &fakeExecutor{observations: append(dirty(2), Observation{Clean: true}), manifest: []byte("m")}
	result, err := Converge(ConvergeRequest{
		Executor:      executor,
		MaxIterations: 5,
		OnApplied:     func(int) { order = append(order, "applied") },
		RecordManifest: func([]byte) error {
			order = append(order, "manifest")
			return nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Iterations)
	assert.Equal(t, []string{"applied", "manifest", "applied", "manifest"}, order)
}

func TestConverge_OnAppliedDoesNotFireForAFailedApply(t *testing.T) {
	applied := 0
	executor := &fakeExecutor{observations: dirty(3), applyErrOn: 2}
	result, err := Converge(ConvergeRequest{
		Executor:      executor,
		MaxIterations: 5,
		OnApplied:     func(int) { applied++ },
	})
	require.Error(t, err)
	assert.Equal(t, OutcomeExecuteError, result.Outcome)
	assert.Equal(t, 1, applied, "only the successful apply is counted against the budget")
}

func TestConverge_LostReceiptStillDominatesAnExhaustedBudget(t *testing.T) {
	// An exhausted budget must not mask a lost receipt: the run mutated the
	// system and cannot prove the result, which outranks why it stopped.
	executor := &fakeExecutor{observations: dirty(3), manifest: []byte("m")}
	result, err := Converge(ConvergeRequest{
		Executor:       executor,
		MaxIterations:  5,
		ApplyBudget:    budgetOf(1),
		RecordManifest: func([]byte) error { return errors.New("catalog unavailable") },
	})
	require.Error(t, err)
	assert.Equal(t, OutcomeBudgetExhausted, result.Outcome)
	assert.True(t, result.NeedsVerification)
}

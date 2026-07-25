package acute

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeExecutor struct {
	observations []Observation
	applyErr     error
	// applyErrOn, when non-zero, fails only that 1-based Apply call. Lets a test
	// script a run that applies successfully, then fails on a later iteration.
	applyErrOn int
	planErr    error
	manifest   []byte
	observed   int
	planned    int
	applied    int
}

func (f *fakeExecutor) Observe() (Observation, error) {
	if f.observed >= len(f.observations) {
		return Observation{}, errors.New("unexpected observe")
	}
	observation := f.observations[f.observed]
	f.observed++
	return observation, nil
}

func (f *fakeExecutor) Plan() (string, error) {
	f.planned++
	if f.planErr != nil {
		return "", f.planErr
	}
	return "plan", nil
}

func (f *fakeExecutor) Apply() ([]byte, error) {
	f.applied++
	if f.applyErrOn != 0 {
		if f.applied == f.applyErrOn {
			return nil, errors.New("provider failed")
		}
		return f.manifest, nil
	}
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	return f.manifest, nil
}

func TestConvergeAppliesUntilVerifiedClean(t *testing.T) {
	fake := &fakeExecutor{
		observations: []Observation{{Clean: false}, {Clean: true}},
		manifest:     []byte(`{"actions":[]}`),
	}
	var recorded [][]byte

	result, err := Converge(ConvergeRequest{
		Executor:       fake,
		MaxIterations:  2,
		RecordManifest: func(manifest []byte) error { recorded = append(recorded, manifest); return nil },
	})

	require.NoError(t, err)
	assert.Equal(t, OutcomeClean, result.Outcome)
	assert.Equal(t, 1, result.Iterations)
	assert.Equal(t, 2, fake.observed)
	assert.Equal(t, 1, fake.applied)
	assert.Equal(t, [][]byte{[]byte(`{"actions":[]}`)}, recorded)
}

func TestConvergeDryRunPlansWithoutApplying(t *testing.T) {
	fake := &fakeExecutor{observations: []Observation{{Clean: false}}}

	result, err := Converge(ConvergeRequest{Executor: fake, MaxIterations: 2, DryRun: true})

	require.NoError(t, err)
	assert.Equal(t, OutcomeDryRun, result.Outcome)
	assert.Equal(t, 1, fake.observed)
	assert.Equal(t, 1, fake.planned)
	assert.Zero(t, fake.applied)
}

func TestConvergeDoesNotReportCleanAfterManifestPersistenceFailure(t *testing.T) {
	fake := &fakeExecutor{
		observations: []Observation{{Clean: false}, {Clean: true}},
		manifest:     []byte(`manifest`),
	}
	var callbackErr error

	result, err := Converge(ConvergeRequest{
		Executor:        fake,
		MaxIterations:   2,
		RecordManifest:  func([]byte) error { return errors.New("catalog unavailable") },
		OnRecordFailure: func(err error) { callbackErr = err },
	})

	require.Error(t, err)
	assert.Equal(t, OutcomeNeedsVerification, result.Outcome)
	assert.EqualError(t, callbackErr, "catalog unavailable")
	assert.Equal(t, 2, fake.observed)
}

func TestConvergeApplyFailureIsNotClean(t *testing.T) {
	fake := &fakeExecutor{
		observations: []Observation{{Clean: false}},
		applyErr:     errors.New("provider failed"),
	}

	result, err := Converge(ConvergeRequest{Executor: fake, MaxIterations: 2})

	require.Error(t, err)
	assert.Equal(t, OutcomeExecuteError, result.Outcome)
	assert.Zero(t, result.Iterations)
}

// A lost receipt used to be dropped on every exit route except the clean one, so
// a cap-exhausted run reported `failed` — the one status that invites re-applying
// work which already succeeded.
func TestConvergeSurfacesLostReceiptWhenCapExhausted(t *testing.T) {
	fake := &fakeExecutor{
		observations: []Observation{{Clean: false}, {Clean: false}},
		manifest:     []byte(`manifest`),
	}

	result, err := Converge(ConvergeRequest{
		Executor:       fake,
		MaxIterations:  1,
		RecordManifest: func([]byte) error { return errors.New("catalog unavailable") },
	})

	require.Error(t, err)
	assert.Equal(t, OutcomeCapExhausted, result.Outcome, "the cap is still why the loop stopped")
	assert.True(t, result.NeedsVerification, "but the lost receipt must survive it")
	assert.Contains(t, err.Error(), "needs verification")
	assert.Equal(t, 1, result.Iterations)
}

// Same masking on the apply-error route: iteration 1 applies and loses its
// receipt, iteration 2's apply then fails. The run must still report that it
// cannot prove what iteration 1 did.
func TestConvergeSurfacesLostReceiptWhenApplyLaterFails(t *testing.T) {
	fake := &fakeExecutor{
		observations: []Observation{{Clean: false}, {Clean: false}},
		manifest:     []byte(`manifest`),
		applyErrOn:   2,
	}
	recorded := 0

	result, err := Converge(ConvergeRequest{
		Executor:      fake,
		MaxIterations: 4,
		RecordManifest: func([]byte) error {
			recorded++
			return errors.New("catalog unavailable")
		},
	})

	require.Error(t, err)
	require.Equal(t, 2, fake.applied, "second apply must have been attempted")
	assert.Equal(t, 1, recorded, "only the successful apply produced a receipt")
	assert.Equal(t, OutcomeExecuteError, result.Outcome, "the failed apply is still why the loop stopped")
	assert.True(t, result.NeedsVerification, "and the earlier lost receipt survives it")
	assert.Contains(t, err.Error(), "needs verification")
	assert.Contains(t, err.Error(), "apply")
	assert.Equal(t, 1, result.Iterations)
}

// Applying and then failing to re-observe is the same operational state as a lost
// receipt: the system changed and PUDL cannot prove the result.
func TestConvergeNeedsVerificationWhenReObserveFails(t *testing.T) {
	fake := &fakeExecutor{
		observations: []Observation{{Clean: false}}, // second Observe errors
		manifest:     []byte(`manifest`),
	}

	result, err := Converge(ConvergeRequest{
		Executor:       fake,
		MaxIterations:  2,
		RecordManifest: func([]byte) error { return nil },
	})

	require.Error(t, err)
	assert.True(t, result.NeedsVerification)
	assert.Equal(t, 1, result.Iterations)
	assert.Contains(t, err.Error(), "re-observation after an apply failed")
}

// A first observation that fails changed nothing, so it is an ordinary failure
// rather than an unverifiable mutation.
func TestConvergeObserveFailureBeforeAnyApplyIsNotNeedsVerification(t *testing.T) {
	fake := &fakeExecutor{observations: nil} // first Observe errors

	result, err := Converge(ConvergeRequest{Executor: fake, MaxIterations: 2})

	require.Error(t, err)
	assert.False(t, result.NeedsVerification)
	assert.Equal(t, OutcomeObserveError, result.Outcome)
	assert.Zero(t, result.Iterations)
}

func TestConvergeStopsAtIterationCap(t *testing.T) {
	fake := &fakeExecutor{
		observations: []Observation{{Clean: false}, {Clean: false}, {Clean: false}},
		manifest:     []byte(`manifest`),
	}

	result, err := Converge(ConvergeRequest{Executor: fake, MaxIterations: 2})

	require.Error(t, err)
	assert.Equal(t, OutcomeCapExhausted, result.Outcome)
	assert.Equal(t, 2, result.Iterations)
	assert.Equal(t, 3, fake.observed)
}

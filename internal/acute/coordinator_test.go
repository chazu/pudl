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
	planErr      error
	manifest     []byte
	observed     int
	planned      int
	applied      int
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

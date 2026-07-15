package acute

import "fmt"

// Observation is the result of one mu observe operation as interpreted by
// PUDL. mu supplies the raw observation; the adapter turns it into this
// lifecycle signal and retains the detailed drift for reporting.
type Observation struct {
	Clean   bool
	Details any
}

// Executor is the narrow seam between PUDL's run policy and mu's execution
// engine. Each method represents one mu operation; PUDL decides ordering and
// stopping, while mu performs the operation itself.
type Executor interface {
	Observe() (Observation, error)
	Plan() (string, error)
	Apply() ([]byte, error)
}

// ManifestRecorder persists mu's apply receipt. A recorder error means the
// external operation may have succeeded but PUDL cannot prove its catalog
// state; the coordinator therefore refuses to return clean.
type ManifestRecorder func([]byte) error

// ObserveHook receives each interpreted observation for progress/reporting.
type ObserveHook func(Observation)

// ApplyHook receives the one-based apply iteration number.
type ApplyHook func(iteration int)

// PlanHook receives the rendered mu plan for a dry run.
type PlanHook func(string)

// Outcome is the terminal lifecycle verdict of a convergence run.
type Outcome string

const (
	OutcomeClean             Outcome = "clean"
	OutcomeCapExhausted      Outcome = "failed (cap_exhausted)"
	OutcomeExecuteError      Outcome = "failed (execute_error)"
	OutcomeDryRun            Outcome = "dry-run (no changes applied)"
	OutcomeNeedsVerification Outcome = "needs-verification"
)

// ConvergeRequest configures one ACUTE convergence loop.
type ConvergeRequest struct {
	Executor        Executor
	MaxIterations   int
	DryRun          bool
	RecordManifest  ManifestRecorder
	OnObserve       ObserveHook
	OnApply         ApplyHook
	OnPlan          PlanHook
	OnRecordFailure func(error)
}

// ConvergeResult contains the coordinator's policy result. Iterations counts
// successful mu Apply operations, not observations.
type ConvergeResult struct {
	Outcome    Outcome
	Iterations int
}

// Converge executes the PUDL-owned observe/apply/re-observe policy around mu.
// A clean result is only possible from an observation after all apply receipts
// have been recorded successfully.
func Converge(request ConvergeRequest) (ConvergeResult, error) {
	if request.Executor == nil {
		return ConvergeResult{}, fmt.Errorf("convergence needs an executor")
	}
	if request.MaxIterations < 1 {
		return ConvergeResult{}, fmt.Errorf("max iterations must be >= 1")
	}

	result := ConvergeResult{}
	manifestFailure := false
	for i := 0; ; i++ {
		observation, err := request.Executor.Observe()
		if err != nil {
			return result, fmt.Errorf("observe: %w", err)
		}
		if request.OnObserve != nil {
			request.OnObserve(observation)
		}

		if observation.Clean {
			result.Outcome = OutcomeClean
			break
		}
		if request.DryRun {
			plan, err := request.Executor.Plan()
			if err != nil {
				return result, fmt.Errorf("plan: %w", err)
			}
			if request.OnPlan != nil {
				request.OnPlan(plan)
			}
			result.Outcome = OutcomeDryRun
			break
		}
		if i >= request.MaxIterations {
			result.Outcome = OutcomeCapExhausted
			break
		}

		iteration := i + 1
		if request.OnApply != nil {
			request.OnApply(iteration)
		}
		manifest, err := request.Executor.Apply()
		if err != nil {
			result.Outcome = OutcomeExecuteError
			return result, fmt.Errorf("apply: %w", err)
		}
		result.Iterations++

		if request.RecordManifest != nil {
			if err := request.RecordManifest(manifest); err != nil {
				manifestFailure = true
				if request.OnRecordFailure != nil {
					request.OnRecordFailure(err)
				}
			}
		}
	}

	if manifestFailure && result.Outcome == OutcomeClean {
		result.Outcome = OutcomeNeedsVerification
		return result, fmt.Errorf("convergence needs verification: an apply manifest was not recorded")
	}
	if result.Outcome == OutcomeCapExhausted {
		return result, fmt.Errorf("convergence %s", result.Outcome)
	}
	return result, nil
}

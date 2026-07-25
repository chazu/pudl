package acute

import "fmt"

// Observation is the result of one mu observe operation as interpreted by
// PUDL. mu supplies the raw observation; the adapter turns it into this
// lifecycle signal and retains the detailed drift for reporting.
type Observation struct {
	Clean bool
	// ObservationID is the catalog entry recording this observation. Empty means
	// the observation was not persisted, so any verdict derived from it cannot be
	// traced to evidence afterwards.
	ObservationID string
	Details       any
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
	OutcomeObserveError      Outcome = "failed (observe_error)"
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

	// NeedsVerification reports that the run changed the live system but cannot
	// prove the resulting state — a manifest receipt was not recorded, or the
	// re-observation after an apply failed. It is deliberately orthogonal to
	// Outcome, because a lost receipt can accompany any terminal outcome, and it
	// must dominate the caller's status decision: reporting `failed` for an apply
	// that actually succeeded invites re-applying it by hand.
	NeedsVerification bool
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

	// Every exit is a `break` rather than a `return` so that the
	// needs-verification verdict below is evaluated on all of them. Returning
	// early from inside the loop is what previously let a lost receipt escape on
	// three of the four exit routes.
	var loopErr error
	for i := 0; ; i++ {
		observation, err := request.Executor.Observe()
		if err != nil {
			result.Outcome = OutcomeObserveError
			loopErr = fmt.Errorf("observe: %w", err)
			break
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
				loopErr = fmt.Errorf("plan: %w", err)
				break
			}
			if request.OnPlan != nil {
				request.OnPlan(plan)
			}
			result.Outcome = OutcomeDryRun
			break
		}
		if i >= request.MaxIterations {
			result.Outcome = OutcomeCapExhausted
			loopErr = fmt.Errorf("convergence %s", OutcomeCapExhausted)
			break
		}

		iteration := i + 1
		if request.OnApply != nil {
			request.OnApply(iteration)
		}
		manifest, err := request.Executor.Apply()
		if err != nil {
			result.Outcome = OutcomeExecuteError
			loopErr = fmt.Errorf("apply: %w", err)
			break
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

	// Two distinct ways to end a run having mutated the system without being able
	// to prove the result: the receipt was lost, or the verifying observation
	// never came back. Both are the same operational state.
	result.NeedsVerification = manifestFailure ||
		(result.Outcome == OutcomeObserveError && result.Iterations > 0)

	if result.NeedsVerification {
		if result.Outcome == OutcomeClean {
			result.Outcome = OutcomeNeedsVerification
		}
		reason := "an apply manifest was not recorded"
		if !manifestFailure {
			reason = "the re-observation after an apply failed"
		}
		if loopErr == nil {
			return result, fmt.Errorf("convergence needs verification: %s", reason)
		}
		return result, fmt.Errorf("convergence needs verification (%s): %w", reason, loopErr)
	}
	return result, loopErr
}

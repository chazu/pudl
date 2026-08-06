package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/mubridge"
	"github.com/chazu/pudl/internal/systemmodel"
)

// planConverge runs `mu build --plan` against the workspace target: it shows the
// actions the converge plugin would apply, executing nothing.
func (w *reconcileWorkspace) planConverge() (string, error) {
	if err := w.activateConfig(w.ConvergeConfig); err != nil {
		return "", err
	}
	out, err := w.Mu.Build(filepath.Join(w.MuRoot, "mu.cue"), w.Target, "--plan", "--json")
	if err != nil {
		return "", fmt.Errorf("mu build --plan %s: %w", w.Target, err)
	}
	return string(out), nil
}

// applyConverge runs `mu build --emit-manifest` against the workspace target: the
// converge plugin applies desired to the live system (kubectl apply, for k8s) and
// the build manifest is emitted as JSON on stdout (chatter + subprocess output go
// to stderr). Returns the manifest bytes so the caller can record per-resource
// status. A non-zero exit is an execute_error (V1.4).
func (w *reconcileWorkspace) applyConverge() ([]byte, error) {
	if err := w.activateConfig(w.ConvergeConfig); err != nil {
		return nil, err
	}
	flags := []string{"--emit-manifest"}
	if w.ExpectedMuPlanSHA256 != "" {
		planned, err := w.planConverge()
		if err != nil {
			return nil, err
		}
		canonical, err := canonicalMutationPlan([]byte(planned), w.Dir)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(canonical)
		actual := hex.EncodeToString(digest[:])
		if actual != w.ExpectedMuPlanSHA256 {
			return nil, fmt.Errorf("approved mu plan changed before apply: expected %s, planned %s", w.ExpectedMuPlanSHA256, actual)
		}
		_, rawDigest, err := validateMuMutationPlan([]byte(planned))
		if err != nil {
			return nil, err
		}
		flags = append(flags, "--expect-plan-sha256", rawDigest)
	}
	return w.Mu.Build(filepath.Join(w.MuRoot, "mu.cue"), w.Target, flags...)
}

// ingestConvergeManifest records the apply's build manifest in the catalog,
// tagged with the model name. Each action lands as a per-resource entry with
// status `converging` (applied, pending verification, build-spec §5); a later
// clean drift re-check promotes those to `clean` via
// promoteConvergingResources -> CatalogDB.PromoteConvergingToCleanByModel. This
// is what wires `pudl run --converge`'s apply into the per-resource lifecycle —
// without it, only the model-level verdict is recorded.
func ingestConvergeManifest(cat *runCatalog, modelName, runID string, manifestJSON []byte) error {
	db, err := cat.optional()
	if err != nil {
		return err
	}
	_, err = mubridge.IngestManifestWithRunID(db, bytes.NewReader(manifestJSON), "mu-build", cat.Dir(), modelName, runID)
	return err
}

// convergeOutcome remains a command-level alias for the ACUTE coordinator's
// lifecycle vocabulary.
type convergeOutcome = acute.Outcome

const (
	outcomeClean             = acute.OutcomeClean
	outcomeCap               = acute.OutcomeCapExhausted
	outcomeExecErr           = acute.OutcomeExecuteError
	outcomeDryRun            = acute.OutcomeDryRun
	outcomeNeedsVerification = acute.OutcomeNeedsVerification
)

type muConvergeExecutor struct {
	workspace *reconcileWorkspace
}

func (e *muConvergeExecutor) Observe() (acute.Observation, error) {
	drift, err := e.workspace.observeDrift()
	if err != nil {
		return acute.Observation{}, err
	}
	return acute.Observation{
		Clean:         drift.Clean,
		ObservationID: drift.ObservationID,
		Details:       drift,
	}, nil
}

func (e *muConvergeExecutor) Plan() (string, error) {
	return e.workspace.planConverge()
}

func (e *muConvergeExecutor) Apply() ([]byte, error) {
	return e.workspace.applyConverge()
}

// runConvergeLoop runs the ACUTE convergence loop against a model: observe drift,
// stop at ∅ (clean) or the iteration cap (failed), otherwise apply and
// re-observe. --dry-run shows the plan and stops (single pass, no mutation).
//
// Loop shape (build-spec §4): fixed-point test at the top, cap as the halting
// guarantee, apply, then re-observe at the next iteration.
func runConvergeLoop(cat *runCatalog, mu muRunner, m *systemmodel.SystemModel, muRoot, modelDir, runID string, maxIters int, dryRun bool, budget *int) (*ConvergeReport, error) {
	return runConvergeLoopExact(cat, mu, m, muRoot, modelDir, runID, maxIters, dryRun, budget, "")
}

func runConvergeLoopExact(cat *runCatalog, mu muRunner, m *systemmodel.SystemModel, muRoot, modelDir, runID string, maxIters int, dryRun bool, budget *int, expectedMuPlanSHA256 string) (*ConvergeReport, error) {
	w, err := setupReconcileWorkspace(cat, mu, m, muRoot, modelDir, runID, dryRun)
	if err != nil {
		return nil, err
	}
	defer w.Cleanup()
	w.ExpectedMuPlanSHA256 = expectedMuPlanSHA256

	live := !jsonOutput // suppress progress chatter when emitting machine JSON
	lastIteration := 0
	var receipts []MutationReceipt
	result, runErr := acute.Converge(acute.ConvergeRequest{
		Executor:      &muConvergeExecutor{workspace: w},
		MaxIterations: maxIters,
		ApplyBudget:   budget,
		DryRun:        dryRun,
		BeforeApply: func(int) error {
			return beginDurableMutationAttempt(cat, runID)
		},
		RecordManifest: func(manifest []byte) error {
			redactedManifest := []byte(redactSealedText(string(manifest), m))
			if err := ingestConvergeManifest(cat, m.Name, runID, redactedManifest); err != nil {
				return err
			}
			if err := completeDurableMutationReceipt(cat, runID); err != nil {
				return err
			}
			digest := sha256.Sum256(manifest)
			receipts = append(receipts, MutationReceipt{
				Iteration: lastIteration, ManifestSHA256: hex.EncodeToString(digest[:]), Status: "completed",
			})
			return nil
		},
		OnObserve: func(observation acute.Observation) {
			if !live {
				return
			}
			if drift, ok := observation.Details.(ModelDriftResult); ok {
				printModelDrift(drift)
			}
		},
		OnPlan: func(plan string) {
			if live {
				fmt.Print("\nplan (dry-run — nothing applied):\n", plan)
			}
		},
		OnApply: func(iteration int) {
			if live {
				fmt.Printf("iteration %d: applying converge…\n", iteration)
			}
		},
		OnApplied: func(iteration int) {
			lastIteration = iteration
			// The durable half of the halting guarantee. Recorded per apply, so a
			// run killed here still tells the next one what it spent.
			recordDurableApply(cat, runID, live)
		},
		OnRecordFailure: func(err error) {
			if live {
				fmt.Printf("warning: per-resource status not recorded: %v\n", err)
			}
		},
	})
	runErr = redactSealedError(runErr, m)

	if runErr != nil && result.Outcome == outcomeExecErr && live {
		fmt.Printf("converge apply failed: %v\n", runErr)
		fmt.Println("WARNING: the live system may be in a partial state — no rollback (V1.5 out of scope).")
	}

	if runErr != nil && result.Outcome == acute.OutcomeBudgetExhausted && live {
		fmt.Println("\nthis model has spent its apply budget since it was last verified clean.")
		fmt.Println("      an unscoped run that observes it clean resets the budget;")
		fmt.Println("      --max-applies 0 disables it for one run.")
	}

	rep := &ConvergeReport{
		Outcome:           string(result.Outcome),
		Iterations:        result.Iterations,
		NeedsVerification: result.NeedsVerification,
		MutationReceipts:  receipts,
	}
	return rep, runErr
}

// recordDurableApply increments the model's spent apply budget. Best-effort in
// the sense that it must not fail a converge mid-flight, but never silent: an
// unrecorded apply is a spent apply the next run will not see, which is how the
// budget would come to over-grant.
func recordDurableApply(cat *runCatalog, runID string, live bool) {
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to record the apply: %v\n", err)
		}
		return
	}
	if err := db.RecordApply(runID); err != nil && live {
		fmt.Printf("warning: could not record the apply against this model's budget: %v\n", err)
	}
}

func beginDurableMutationAttempt(cat *runCatalog, runID string) error {
	db, err := cat.required()
	if err != nil {
		return err
	}
	return db.BeginRunMutationAttempt(runID)
}

func completeDurableMutationReceipt(cat *runCatalog, runID string) error {
	db, err := cat.required()
	if err != nil {
		return err
	}
	return db.CompleteRunMutationReceipt(runID)
}

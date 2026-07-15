package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/mubridge"
	"github.com/chazu/pudl/internal/systemmodel"
)

// planConverge runs `mu build --plan` against the workspace target: it shows the
// actions the converge plugin would apply, executing nothing.
func (w *reconcileWorkspace) planConverge() (string, error) {
	cmd := exec.Command("mu", "build", "--plan", "--config", filepath.Join(w.MuRoot, "mu.cue"), w.Target)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mu build --plan %s: %w: %s", w.Target, err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// applyConverge runs `mu build --emit-manifest` against the workspace target: the
// converge plugin applies desired to the live system (kubectl apply, for k8s) and
// the build manifest is emitted as JSON on stdout (chatter + subprocess output go
// to stderr). Returns the manifest bytes so the caller can record per-resource
// status. A non-zero exit is an execute_error (V1.4).
func (w *reconcileWorkspace) applyConverge() ([]byte, error) {
	cmd := exec.Command("mu", "build", "--emit-manifest", "--config", filepath.Join(w.MuRoot, "mu.cue"), w.Target)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// ingestConvergeManifest records the apply's build manifest in the catalog,
// tagged with the model name. Each action lands as a per-resource entry with
// status `converging` (applied, pending verification, build-spec §5); a later
// clean drift re-check promotes those to `clean` via
// promoteConvergingResources -> CatalogDB.PromoteConvergingToCleanByModel. This
// is what wires `pudl run --converge`'s apply into the per-resource lifecycle —
// without it, only the model-level verdict is recorded.
func ingestConvergeManifest(modelName, runID string, manifestJSON []byte) error {
	pudlDir := config.GetPudlDir()
	db, err := database.NewCatalogDB(pudlDir)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = mubridge.IngestManifestWithRunID(db, bytes.NewReader(manifestJSON), "mu-build", pudlDir, modelName, runID)
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
	return acute.Observation{Clean: drift.Clean, Details: drift}, nil
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
func runConvergeLoop(m *systemmodel.SystemModel, muRoot, modelDir, runID string, maxIters int, dryRun bool) (*ConvergeReport, error) {
	w, err := setupReconcileWorkspace(m, muRoot, modelDir)
	if err != nil {
		return nil, err
	}
	defer w.Cleanup()

	live := !jsonOutput // suppress progress chatter when emitting machine JSON
	result, runErr := acute.Converge(acute.ConvergeRequest{
		Executor:      &muConvergeExecutor{workspace: w},
		MaxIterations: maxIters,
		DryRun:        dryRun,
		RecordManifest: func(manifest []byte) error {
			return ingestConvergeManifest(m.Name, runID, manifest)
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
		OnRecordFailure: func(err error) {
			if live {
				fmt.Printf("warning: per-resource status not recorded: %v\n", err)
			}
		},
	})

	if runErr != nil && result.Outcome == outcomeExecErr && live {
		fmt.Printf("converge apply failed: %v\n", runErr)
		fmt.Println("WARNING: the live system may be in a partial state — no rollback (V1.5 out of scope).")
	}

	rep := &ConvergeReport{Outcome: string(result.Outcome), Iterations: result.Iterations}
	return rep, runErr
}

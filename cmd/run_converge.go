package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

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

// applyConverge runs `mu build` against the workspace target: the converge plugin
// applies desired to the live system (kubectl apply, for k8s). A non-zero exit is
// an execute_error (V1.4).
func (w *reconcileWorkspace) applyConverge() error {
	cmd := exec.Command("mu", "build", "--config", filepath.Join(w.MuRoot, "mu.cue"), w.Target)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(errb.String()))
	}
	return nil
}

// convergeOutcome is the terminal verdict of a converge run.
type convergeOutcome string

const (
	outcomeConverged convergeOutcome = "converged"
	outcomeCap       convergeOutcome = "failed (cap_exhausted)"
	outcomeExecErr   convergeOutcome = "failed (execute_error)"
	outcomeDryRun    convergeOutcome = "dry-run (no changes applied)"
)

// runConvergeLoop runs the ACUTE convergence loop against a model: observe drift,
// stop at ∅ (converged) or the iteration cap (failed), otherwise apply and
// re-observe. --dry-run shows the plan and stops (single pass, no mutation).
//
// Loop shape (build-spec §4): fixed-point test at the top, cap as the halting
// guarantee, apply, then re-observe at the next iteration.
func runConvergeLoop(m *systemmodel.SystemModel, muRoot, modelDir string, maxIters int, dryRun bool) error {
	w, err := setupReconcileWorkspace(m, muRoot, modelDir)
	if err != nil {
		return err
	}
	defer w.Cleanup()

	var outcome convergeOutcome
	for i := 0; ; i++ {
		drift, err := w.observeDrift()
		if err != nil {
			return err
		}
		printModelDrift(drift)

		if drift.Clean {
			outcome = outcomeConverged
			break
		}
		if dryRun {
			plan, err := w.planConverge()
			if err != nil {
				return err
			}
			fmt.Print("\nplan (dry-run — nothing applied):\n", plan)
			outcome = outcomeDryRun
			break
		}
		if i >= maxIters {
			outcome = outcomeCap
			break
		}
		fmt.Printf("iteration %d: applying converge…\n", i+1)
		if err := w.applyConverge(); err != nil {
			fmt.Printf("converge apply failed: %v\n", err)
			fmt.Println("WARNING: the live system may be in a partial state — no rollback (V1.5 out of scope).")
			outcome = outcomeExecErr
			break
		}
	}

	fmt.Printf("\nresult: %s\n", outcome)
	if outcome == outcomeCap || outcome == outcomeExecErr {
		return fmt.Errorf("convergence %s", outcome)
	}
	return nil
}

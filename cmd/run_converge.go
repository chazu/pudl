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
	outcomeClean   convergeOutcome = "clean"
	outcomeCap     convergeOutcome = "failed (cap_exhausted)"
	outcomeExecErr convergeOutcome = "failed (execute_error)"
	outcomeDryRun  convergeOutcome = "dry-run (no changes applied)"
)

// runConvergeLoop runs the ACUTE convergence loop against a model: observe drift,
// stop at ∅ (clean) or the iteration cap (failed), otherwise apply and
// re-observe. --dry-run shows the plan and stops (single pass, no mutation).
//
// Loop shape (build-spec §4): fixed-point test at the top, cap as the halting
// guarantee, apply, then re-observe at the next iteration.
func runConvergeLoop(m *systemmodel.SystemModel, muRoot, modelDir string, maxIters int, dryRun bool) (*ConvergeReport, error) {
	w, err := setupReconcileWorkspace(m, muRoot, modelDir)
	if err != nil {
		return nil, err
	}
	defer w.Cleanup()

	live := !jsonOutput // suppress progress chatter when emitting machine JSON
	var outcome convergeOutcome
	applies := 0
	for i := 0; ; i++ {
		drift, err := w.observeDrift()
		if err != nil {
			return nil, err
		}
		if live {
			printModelDrift(drift)
		}

		if drift.Clean {
			outcome = outcomeClean
			break
		}
		if dryRun {
			plan, err := w.planConverge()
			if err != nil {
				return nil, err
			}
			if live {
				fmt.Print("\nplan (dry-run — nothing applied):\n", plan)
			}
			outcome = outcomeDryRun
			break
		}
		if i >= maxIters {
			outcome = outcomeCap
			break
		}
		if live {
			fmt.Printf("iteration %d: applying converge…\n", i+1)
		}
		if err := w.applyConverge(); err != nil {
			if live {
				fmt.Printf("converge apply failed: %v\n", err)
				fmt.Println("WARNING: the live system may be in a partial state — no rollback (V1.5 out of scope).")
			}
			outcome = outcomeExecErr
			break
		}
		applies++
	}

	rep := &ConvergeReport{Outcome: string(outcome), Iterations: applies}
	if outcome == outcomeCap || outcome == outcomeExecErr {
		return rep, fmt.Errorf("convergence %s", outcome)
	}
	return rep, nil
}

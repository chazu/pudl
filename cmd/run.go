package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/systemmodel"
)

var (
	runFile     string
	runMuRoot   string
	runConverge bool
	runOnly     []string
	runDryRun   bool
	runMaxIters int
)

var runCmd = &cobra.Command{
	Use:   "run <model>",
	Short: "Run a #SystemModel instance (observe-only, or --converge)",
	Long: `Run a #SystemModel instance through the ACUTE cycle.

<model> is the instance name (the CUE field), loaded from --file
(default: models/<model>.cue). Default is OBSERVE-ONLY: populate -> drift ->
checks -> report, no mutation. Pass --converge to close drift; see the V1
build spec.

Examples:
    pudl run k8sPolicy
    pudl run k8sPolicy --file models/k8s.cue
    pudl run k8sConverge --converge
    pudl run k8sConverge --converge --only web,api
    pudl run k8sConverge --converge --dry-run`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		flags := runFlags{
			converge: runConverge,
			only:     runOnly,
			dryRun:   runDryRun,
			maxIters: runMaxIters,
			// whether a convergence flag was explicitly set (for the gate rules)
			onlySet:     cmd.Flags().Changed("only"),
			dryRunSet:   cmd.Flags().Changed("dry-run"),
			maxItersSet: cmd.Flags().Changed("max-iters"),
		}
		if err := validateRunFlags(flags); err != nil {
			return err
		}

		file := runFile
		if file == "" {
			file = fmt.Sprintf("models/%s.cue", name)
		}
		model, err := systemmodel.LoadModelFile(file, name)
		if err != nil {
			return fmt.Errorf("load model: %w", err)
		}

		fmt.Print(buildRunPlan(model, flags))

		modelDir := filepath.Dir(file)
		muRoot := runMuRoot
		if muRoot == "" {
			muRoot, err = findMuRoot(modelDir)
			if err != nil {
				return err
			}
		}

		// Convergence is its own loop (mutates; closes drift) — return early.
		if flags.converge && model.Convergent() {
			fmt.Println("\n— converge —")
			return runConvergeLoop(model, muRoot, modelDir, flags.maxIters, flags.dryRun)
		}

		// Observe-only paths. A model with `desired` flags drift via the
		// differential path (read-only here); without it, inventory populate.
		if len(model.Desired) > 0 {
			fmt.Println("\n— drift —")
			res, err := runDrift(model, muRoot, modelDir)
			if err != nil {
				return err
			}
			printModelDrift(res)
		} else {
			fmt.Println("\n— populate —")
			if err := runPopulate(model, muRoot, modelDir); err != nil {
				return err
			}
		}

		// CHECK — evaluate the model's Datalog checks over the catalog.
		if len(model.Checks) > 0 {
			fmt.Println("\n— checks —")
			results, err := runChecks(model, modelDir)
			if err != nil {
				return err
			}
			if printChecks(results) {
				return fmt.Errorf("one or more fail-severity checks did not pass")
			}
		}
		return nil
	},
}

// printModelDrift renders a model-level drift verdict.
func printModelDrift(r ModelDriftResult) {
	if r.Clean {
		fmt.Println("drift: ∅ (converged — all desired resources exist and match)")
		return
	}
	fmt.Printf("drift: %d resource(s)\n", len(r.Drifted))
	for _, d := range r.Drifted {
		if d.Diff != "" {
			fmt.Printf("  ~ %s (%s): %s\n", d.Resource, d.Reason, d.Diff)
		} else {
			fmt.Printf("  ~ %s (%s)\n", d.Resource, d.Reason)
		}
	}
}

// runFlags is the validated CLI surface for `pudl run`.
type runFlags struct {
	converge bool
	only     []string
	dryRun   bool
	maxIters int

	onlySet     bool
	dryRunSet   bool
	maxItersSet bool
}

// validateRunFlags enforces the gate rules: convergence flags require --converge.
// One rule — convergence flags need the convergence gate — so a resource can't
// be named (or a plan dry-run requested) without explicitly opting into mutation.
func validateRunFlags(f runFlags) error {
	if f.converge {
		if f.maxIters < 1 {
			return fmt.Errorf("--max-iters must be >= 1")
		}
		return nil
	}
	switch {
	case f.onlySet:
		return fmt.Errorf("--only requires --converge")
	case f.dryRunSet:
		return fmt.Errorf("--dry-run requires --converge")
	case f.maxItersSet:
		return fmt.Errorf("--max-iters requires --converge")
	}
	return nil
}

// buildRunPlan renders the resolved phase plan for a model under the given flags.
// Observe-only when --converge is absent (or the model declares no converge arm);
// the converge loop otherwise.
func buildRunPlan(m *systemmodel.SystemModel, f runFlags) string {
	var b strings.Builder
	fmt.Fprintf(&b, "model:    %s\n", m.Name)
	fmt.Fprintf(&b, "populate: %s (%s)\n", m.Populate.Kind(), populateRef(m.Populate))
	fmt.Fprintf(&b, "checks:   %d\n", len(m.Checks))

	mode := "observe-only"
	if f.converge {
		if !m.Convergent() {
			mode = "observe-only (model declares no converge arm; --converge is a no-op)"
		} else {
			mode = fmt.Sprintf("converge via %q (max-iters %d)", m.Converge.Plugin, f.maxIters)
			if f.dryRun {
				mode += ", dry-run (plan only, no execute, no status writes)"
			}
			if len(f.only) > 0 {
				mode += fmt.Sprintf(", only: %s", strings.Join(f.only, ","))
			}
		}
	}
	fmt.Fprintf(&b, "mode:     %s\n", mode)

	fmt.Fprintln(&b, "\nphases:")
	fmt.Fprintln(&b, "  1. populate -> ingest (Accumulate)")
	fmt.Fprintln(&b, "  2. drift             (Unify)")
	fmt.Fprintln(&b, "  3. checks            (flag)")
	fmt.Fprintln(&b, "  4. report")
	if f.converge && m.Convergent() {
		fmt.Fprintln(&b, "  loop: drift==∅ -> converged | cap -> failed | else converge->execute->re-observe")
	}
	return b.String()
}

// populateRef returns a short identifier for the populate arm (plugin name or
// ewe source).
func populateRef(p systemmodel.Populate) string {
	if p.Kind() == systemmodel.KindEweTarget {
		return p.EweSource
	}
	return p.Plugin
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&runFile, "file", "", "model source file (default: models/<model>.cue)")
	runCmd.Flags().StringVar(&runMuRoot, "mu-root", "", "mu project root to run within (default: discover mu.cue from the model dir)")
	runCmd.Flags().BoolVar(&runConverge, "converge", false, "opt into the convergence loop (mutates the target)")
	runCmd.Flags().StringSliceVar(&runOnly, "only", nil, "converge only these definitions (requires --converge)")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "print the plan, execute nothing (requires --converge)")
	runCmd.Flags().IntVar(&runMaxIters, "max-iters", 5, "loop iteration cap (requires --converge)")
}

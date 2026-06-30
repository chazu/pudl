package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

var (
	runMuRoot      string
	runConverge    bool
	runOnly        []string
	runDryRun      bool
	runMaxIters    int
	runFromCatalog bool
)

var runCmd = &cobra.Command{
	Use:   "run <model>",
	Short: "Run a #SystemModel instance (observe-only, or --converge)",
	Long: `Run a #SystemModel instance through the ACUTE cycle.

<model> is a registered #SystemModel — a definition inheriting #SystemModel,
resolved by name (its name field or short definition name) from the project
.pudl/schema first, then the global ~/.pudl/schema. Register one with
"pudl schema add". Default is OBSERVE-ONLY: populate -> drift -> checks ->
report, no mutation. Pass --converge to close drift; see the V1 build spec.

Examples:
    pudl run github-chazu
    pudl run k8sPolicy --converge
    pudl run k8sConverge --converge --only web,api
    pudl run k8sConverge --converge --dry-run`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		flags := runFlags{
			converge:    runConverge,
			only:        runOnly,
			dryRun:      runDryRun,
			maxIters:    runMaxIters,
			fromCatalog: runFromCatalog,
			// whether a convergence flag was explicitly set (for the gate rules)
			onlySet:     cmd.Flags().Changed("only"),
			dryRunSet:   cmd.Flags().Changed("dry-run"),
			maxItersSet: cmd.Flags().Changed("max-iters"),
		}
		if err := validateRunFlags(flags); err != nil {
			return err
		}

		// Resolve the model from the registered schemas (project .pudl/schema
		// wins over global ~/.pudl/schema). modelDir is where it was loaded from
		// — the base for eweSource + relative plugin paths.
		model, modelDir, pudlRoot, err := resolveModel(name)
		if err != nil {
			return err
		}

		live := !jsonOutput
		if live {
			fmt.Print(buildRunPlan(model, flags))
		}

		// Record the instance in the catalog (identity = name) so every model
		// that's been run is inventoriable via `pudl list`/`query`. Best-effort:
		// a recording failure must not fail the run.
		if err := recordModelInstance(model); err != nil && live {
			fmt.Printf("warning: could not record model instance: %v\n", err)
		}

		// muRoot is only needed by paths that run mu within an existing project
		// (plugin-observe live observe; differential drift). The ewe populate
		// path self-stages its own mu project, and --from-catalog runs no mu.
		// Best-effort: phases that genuinely need it validate when they run.
		muRoot := runMuRoot
		if muRoot == "" && !flags.fromCatalog {
			muRoot, _ = findMuRoot(modelDir)
		}

		if flags.fromCatalog && len(model.Desired) == 0 {
			return fmt.Errorf("--from-catalog needs desired state; model %q declares none", model.Name)
		}

		report := &RunReport{Model: model.Name, OK: true}
		var runErr error

		switch {
		case flags.converge && model.Convergent():
			report.Mode = "converge"
			if flags.dryRun {
				report.Mode = "dry-run"
			}
			if live {
				fmt.Println("\n— converge —")
			}
			cr, err := runConvergeLoop(model, muRoot, modelDir, flags.maxIters, flags.dryRun)
			report.Converge = cr
			if err != nil {
				report.OK = false
				runErr = err
			}

		default:
			report.Mode = "observe-only"
			// A model with `desired` flags drift; without it, populate.
			switch {
			case len(model.Desired) > 0 && useInventoryDrift(model, flags.fromCatalog):
				// Inventory: set-diff desired vs already-ingested catalog records
				// (no live observe). Auto-selected for inventory observers
				// (EweTarget, or #PluginObserve differential:false); --from-catalog
				// forces it for any model.
				report.Mode = "observe-only (inventory)"
				db, err := database.NewCatalogDB(config.GetPudlDir())
				if err != nil {
					return fmt.Errorf("open catalog: %w", err)
				}
				defer db.Close()
				identity, err := schemaIdentityResolver()
				if err != nil {
					return err
				}
				res, err := runInventoryDrift(db, "", model.Desired, identity)
				if err != nil {
					return err
				}
				report.Drift = &res
			case len(model.Desired) > 0:
				// Differential: live observe with desired-as-sources (k8s-style).
				res, err := runDrift(model, muRoot, modelDir)
				if err != nil {
					return err
				}
				report.Drift = &res
			default:
				pr, err := runPopulate(model, muRoot, modelDir, pudlRoot)
				if err != nil {
					return err
				}
				report.Populate = pr
			}
			if len(model.Checks) > 0 {
				results, err := runChecks(model, modelDir)
				if err != nil {
					return err
				}
				report.Checks = results
				if anyFailSeverityFailed(results) {
					report.OK = false
					runErr = fmt.Errorf("one or more fail-severity checks did not pass")
				}
			}
		}

		out, err := report.render(jsonOutput)
		if err != nil {
			return err
		}
		if live {
			fmt.Print("\n")
		}
		fmt.Print(out)

		// Persist the run's terminal verdict on the model instance row so
		// `pudl model list` / `pudl status` surface last-run state.
		persistRunStatus(model.Name, runVerdict(report, flags))

		// A clean drift re-check verifies any pending apply: promote this model's
		// resources from `converging` (written by ingest-manifest) to `clean`.
		if report.Drift != nil && report.Drift.Clean && !flags.dryRun {
			promoteConvergingResources(model)
		}
		return runErr
	},
}

// runVerdict maps a finished run to a catalog status, or "" when none applies:
// dry-run writes nothing (build-spec §3) and a pure populate has no drift verdict.
//
// "clean" is the single in-sync verdict (drift == ∅) — written whether the model
// is observe-only or was just converged, since the convergence loop ends in the
// same re-observed ∅ state. It is only ever written off an actual ∅ observation.
func runVerdict(r *RunReport, f runFlags) string {
	if f.dryRun {
		return ""
	}
	switch {
	case r.Converge != nil:
		if r.Converge.Outcome == string(outcomeClean) {
			return "clean"
		}
		if strings.HasPrefix(r.Converge.Outcome, "failed") {
			return "failed"
		}
		return ""
	case r.Drift != nil:
		if r.Drift.Clean {
			return "clean"
		}
		return "drifted"
	default:
		return ""
	}
}

// persistRunStatus records a run verdict on the model instance row
// (target = modelTargetKey(name)). Best-effort: a status-write failure (or no
// catalog) never fails the run.
func persistRunStatus(name, status string) {
	if status == "" {
		return
	}
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return
	}
	defer db.Close()
	_ = db.UpdateStatus(modelTargetKey(name), status)
}

// promoteConvergingResources flips this model's resources from `converging` to
// `clean` after a verified clean drift (the drift re-check confirming a pending
// apply). Best-effort: a missing catalog/resolver never fails the run. Scoped to
// the model's own resource definition names, so it cannot touch another model's
// pending resources.
func promoteConvergingResources(m *systemmodel.SystemModel) {
	if len(m.Desired) == 0 {
		return
	}
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return
	}
	defer db.Close()

	// Exact path: rows tagged with this model by `ingest-manifest --model <name>`.
	if n, err := db.PromoteConvergingToCleanByModel(m.Name); err == nil && n > 0 {
		return
	}

	// Fallback (manifests ingested without --model): derive candidate resource
	// definition names from the model's desired records and promote matches.
	identity, err := schemaIdentityResolver()
	if err != nil {
		return
	}
	defs := modelResourceDefs(m.Desired, identity)
	if len(defs) == 0 {
		return
	}
	_, _ = db.PromoteConvergingToClean(defs)
}

// useInventoryDrift decides the drift computation for a model with desired state:
// inventory set-diff (against catalog records, no live observe) vs a differential
// live observe. Inventory when --from-catalog is forced, or the model's observer is
// not differential (EweTarget, or #PluginObserve differential:false).
func useInventoryDrift(m *systemmodel.SystemModel, fromCatalog bool) bool {
	return fromCatalog || !m.DifferentialDrift()
}

// anyFailSeverityFailed reports whether any severity:"fail" check did not pass.
func anyFailSeverityFailed(results []CheckResult) bool {
	for _, c := range results {
		if !c.Passed && c.Severity == "fail" {
			return true
		}
	}
	return false
}

// printModelDrift renders a model-level drift verdict.
func printModelDrift(r ModelDriftResult) {
	if r.Clean {
		fmt.Println("drift: ∅ (clean — all desired resources exist and match)")
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
	converge    bool
	only        []string
	dryRun      bool
	maxIters    int
	fromCatalog bool

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
		fmt.Fprintln(&b, "  loop: drift==∅ -> clean | cap -> failed | else converge->execute->re-observe")
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
	runCmd.Flags().StringVar(&runMuRoot, "mu-root", "", "mu project root to run within (default: discover mu.cue from the model dir)")
	runCmd.Flags().BoolVar(&runConverge, "converge", false, "opt into the convergence loop (mutates the target)")
	runCmd.Flags().StringSliceVar(&runOnly, "only", nil, "converge only these definitions (requires --converge)")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "print the plan, execute nothing (requires --converge)")
	runCmd.Flags().IntVar(&runMaxIters, "max-iters", 5, "loop iteration cap (requires --converge)")
	runCmd.Flags().BoolVar(&runFromCatalog, "from-catalog", false, "drift over already-ingested records (inventory; no live observe)")
}

package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

var (
	runMuRoot        string
	runConverge      bool
	runOnly          []string
	runDryRun        bool
	runMaxIters      int
	runFromCatalog   bool
	runCatalogScope  string
	runCheckUpstream bool
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
	RunE: func(cmd *cobra.Command, args []string) (runError error) {
		name := args[0]

		flags := runFlags{
			converge:     runConverge,
			only:         runOnly,
			dryRun:       runDryRun,
			maxIters:     runMaxIters,
			fromCatalog:  runFromCatalog,
			catalogScope: runCatalogScope,
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
		plan, err := acute.NewRunPlan(model, acute.RunRequest{
			Converge:    flags.converge,
			Only:        flags.only,
			DryRun:      flags.dryRun,
			MaxIters:    flags.maxIters,
			FromCatalog: flags.fromCatalog,
		})
		if err != nil {
			return err
		}
		session := acute.NewRunSession(plan)
		effectiveModel := session.Plan.Effective

		live := !jsonOutput
		if live {
			fmt.Print(renderRunPlan(plan))
		}

		// Audit the run for real, from here on. A dry run is exempt because it must
		// not touch catalog state at all.
		mode := "observe-only"
		if flags.converge && model.Convergent() {
			mode = "converge"
		}
		// finishState is populated once the run concludes and read by the deferred
		// finalizer below.
		finishState := &runFinishState{}
		if !flags.dryRun {
			startRunRecord(session.RunID, model.Name, mode, live)

			// The finalizer runs on every exit path, including an early `return err`,
			// so a run that ends badly is still recorded as *ended*. A row left
			// unfinished therefore means the process died without a word.
			defer func() { finishRunRecord(session.RunID, *finishState, runError, live) }()

			// A converge run can mutate before it is able to write a verdict, so the
			// model's previous verdict stops being trustworthy the moment it starts.
			// Clearing it to `unknown` up front means a crashed converge leaves
			// `unknown` rather than a stale `clean`. Observe-only runs change nothing,
			// so their model keeps its last real verdict.
			if mode == "converge" {
				persistRunStatus(model.Name, "unknown", live)
			}
		}

		// A dry run must not mutate catalog state, memberships, facts or statuses:
		// it is documented as showing what *would* happen. Both writes below are
		// real mutations, and both used to run unconditionally — before the
		// dry-run branch was ever reached — so `--dry-run` created snapshot
		// entries, item entries, collection memberships, raw files and
		// model_depends_on facts.
		if !flags.dryRun {
			// Record the instance in the catalog (identity = name) so every model
			// that's been run is inventoriable via `pudl list`/`query`. Best-effort:
			// a recording failure must not fail the run.
			if err := recordModelInstance(model, session.RunID); err != nil && live {
				fmt.Printf("warning: could not record model instance: %v\n", err)
			}

			// Reconcile this model's declared depends_on into model_depends_on facts
			// (add new edges, invalidate removed ones). Best-effort: a reconcile
			// failure must not fail the run. Warnings (e.g. unresolved deps) surface.
			if warns, err := reconcileModelDependencies(model); err != nil {
				if live {
					fmt.Printf("warning: could not reconcile dependencies: %v\n", err)
				}
			} else if live {
				for _, w := range warns {
					fmt.Printf("warning: %s\n", w)
				}
			}
		} else if live {
			fmt.Println("dry-run: skipping model-instance and dependency-fact writes")
		}

		// Opt-in stale-input guard: warn if any transitive upstream is drifted/failed.
		if runCheckUpstream && live {
			for _, w := range checkUpstreamFreshness(model) {
				fmt.Printf("warning: %s\n", w)
			}
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

		report := &RunReport{RunID: session.RunID, Model: model.Name, OK: true}
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
			cr, err := runConvergeLoop(effectiveModel, muRoot, modelDir, session.RunID, flags.maxIters, flags.dryRun)
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
				identity, err := schemaIdentityResolver()
				if err != nil {
					return err
				}
				// Scope is mandatory on both arms: a live run compares against the
				// snapshot it just populated, and a replay compares against the
				// scope the operator named (validateRunFlags requires one). An
				// empty scope would query every observe record in the catalog.
				var scope string
				if flags.fromCatalog {
					scope = strings.TrimSpace(flags.catalogScope)
				} else {
					pr, err := runPopulate(model, muRoot, modelDir, pudlRoot, session.RunID)
					if err != nil {
						return err
					}
					report.Populate = pr
					scope = pr.SnapshotID
					if scope == "" {
						return fmt.Errorf("populate produced no snapshot to compare against")
					}
				}
				// The catalog is opened *after* populate, not before. Opening it up
				// front left this reader handle open across runPopulate, which opens
				// its own handle and writes the very records this then reads —
				// a second connection writing under an open reader, for no benefit,
				// since nothing here reads the catalog until populate has finished.
				db, err := database.NewCatalogDB(config.GetPudlDir())
				if err != nil {
					return fmt.Errorf("open catalog: %w", err)
				}
				defer db.Close()
				res, err := runInventoryDrift(db, scope, model.Desired, identity)
				if err != nil {
					return err
				}
				// A replay is not an observation of the live system, so its verdict
				// cannot promote resources or write a clean status.
				res.Verified = !flags.fromCatalog
				report.Drift = &res
			case len(model.Desired) > 0:
				// Differential: live observe with desired-as-sources (k8s-style).
				res, err := runDrift(model, muRoot, modelDir, session.RunID)
				if err != nil {
					return err
				}
				report.Drift = &res
			default:
				pr, err := runPopulate(model, muRoot, modelDir, pudlRoot, session.RunID)
				if err != nil {
					return err
				}
				report.Populate = pr
			}
			// Checks read the effective scoped model, not the original: invariant 2
			// requires one model shape across planning, execution, report scope,
			// promotion and scope-sensitive checks. Handing checks the unscoped
			// model would let a check assert over resources this run excluded.
			if len(effectiveModel.Checks) > 0 {
				results, err := runChecks(effectiveModel, modelDir)
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
		// `pudl model list` / `pudl status` surface last-run state, and record the
		// same conclusion on the run row. The run row is what tells an `unknown`
		// caused by a lost receipt apart from the `unknown` of a resource nobody has
		// ever observed — they are the same value on the model row by design.
		verdict := runVerdict(report, flags)
		finishState.verdict = verdict
		if report.Converge != nil {
			finishState.outcome = report.Converge.Outcome
			finishState.needsVerification = report.Converge.NeedsVerification
		}

		// The run row keeps the run's real verdict; the model instance row
		// describes the *whole* model, so a scoped run's verdict may not be
		// generalizable onto it. The note records the divergence so a reader can
		// tell this `unknown` from one caused by a lost receipt.
		restricted := len(flags.only) > 0
		rowVerdict := modelRowVerdict(verdict, restricted)
		if rowVerdict != verdict {
			finishState.note = fmt.Sprintf("verdict %q covers only the --only scope (%s); model status left %q",
				verdict, strings.Join(flags.only, ","), rowVerdict)
			if live {
				fmt.Printf("\nnote: %s\n", finishState.note)
				fmt.Println("      a scoped ∅ does not prove the whole model clean; re-run unscoped to establish it")
			}
		}
		persistRunStatus(model.Name, rowVerdict, live)

		// A verified ∅ re-check promotes this model's resources from `converging`
		// (written by the apply's ingest-manifest, or a prior ingest-manifest run)
		// to `clean`. The ∅ comes from the converge loop's final re-observe
		// (report.Converge clean) or an observe-only drift (report.Drift clean).
		// Drift.Verified is load-bearing: a clean `--from-catalog` replay says the
		// desired set matches *recorded* records, which may predate the last apply.
		// Promoting off that would satisfy invariant 5 in name only.
		verifiedClean := !flags.dryRun &&
			((report.Drift != nil && report.Drift.Clean && report.Drift.Verified) ||
				(report.Converge != nil && report.Converge.Outcome == string(outcomeClean)))
		if verifiedClean {
			promoteConvergingResources(effectiveModel, restricted)
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
		// Checked before the outcome: a run that mutated the system without being
		// able to prove the result is `unknown` however the loop ended. Falling
		// through to `failed` here would describe an apply that succeeded as one
		// that did not, inviting a manual re-apply.
		if r.Converge.NeedsVerification || r.Converge.Outcome == string(outcomeNeedsVerification) {
			return "unknown"
		}
		if r.Converge.Outcome == string(outcomeClean) {
			return "clean"
		}
		if strings.HasPrefix(r.Converge.Outcome, "failed") {
			return "failed"
		}
		return ""
	case r.Drift != nil:
		// An unverified verdict (a `--from-catalog` replay) observed nothing, so it
		// records nothing: writing `clean` would be false, and writing `drifted`
		// off records that may be stale would be no better. The model keeps the
		// verdict of its last real observation.
		if !r.Drift.Verified {
			return ""
		}
		if r.Drift.Clean {
			return "clean"
		}
		return "drifted"
	default:
		return ""
	}
}

// modelRowVerdict maps a run's verdict onto the model instance row, which
// describes the model as a whole.
//
// Under `--only` the run planned, executed and observed a subset, so its verdict
// is a statement about that subset. Only `clean` fails to generalize: a ∅ over
// the named resources says nothing about the ones excluded from scope, and
// writing it to the model row would let `pudl status`, `pudl model list` and —
// worst — checkUpstreamFreshness read a whole-model "in sync" off a partial run.
// The remaining verdicts survive the generalization intact: drift or a failure in
// a subset *is* drift or a failure in the model, and `unknown` is already the
// weakest claim available. A non-generalizable `clean` therefore degrades to
// `unknown`, which is what the model's whole-model state genuinely is.
func modelRowVerdict(verdict string, restricted bool) string {
	if restricted && verdict == "clean" {
		return "unknown"
	}
	return verdict
}

// persistRunStatus records a run verdict on the model instance row
// (target = modelTargetKey(name)). Best-effort: a status-write failure (or no
// catalog) never fails the run, but it is reported rather than swallowed — a
// silently dropped verdict leaves the previous run's status standing, which is
// how a stale `clean` used to survive.
func persistRunStatus(name, status string, live bool) {
	if status == "" {
		return
	}
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to record status %q: %v\n", status, err)
		}
		return
	}
	defer db.Close()
	if err := db.UpdateStatus(modelTargetKey(name), status); err != nil && live {
		fmt.Printf("warning: could not record status %q: %v\n", status, err)
	}
}

// runFinishState is what a completed run concluded, handed to the deferred
// finalizer so every exit path — including an early `return err` — records a
// terminal run row.
type runFinishState struct {
	verdict           string
	outcome           string
	needsVerification bool
	// note explains a verdict whose meaning is not evident from the value alone
	// — currently a scoped run whose model row diverges from its run row.
	note string
}

// startRunRecord opens the run's audit row before any phase runs, and reports any
// earlier run of this model that never finished. An unfinished row means a prior
// invocation died without recording a verdict, so the status that model currently
// carries predates it. Best-effort: auditing must not fail the run.
func startRunRecord(runID, model, mode string, live bool) {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to record the run: %v\n", err)
		}
		return
	}
	defer db.Close()

	if stale, err := db.UnfinishedRuns(model); err == nil && len(stale) > 0 && live {
		fmt.Printf("warning: %d earlier run(s) of %q never finished (most recent: %s, started %s)\n",
			len(stale), model, stale[0].RunID, stale[0].StartedAt.Format(time.RFC3339))
		fmt.Println("         that model's recorded status predates those runs and may be stale")
	}
	if err := db.StartRun(runID, model, mode); err != nil && live {
		fmt.Printf("warning: could not record run start: %v\n", err)
	}
}

// finishRunRecord marks the run terminal. It runs from a defer so that an early
// error return is still a *recorded* termination — distinguishable from a process
// that died without saying anything, which leaves the row unfinished.
func finishRunRecord(runID string, state runFinishState, runErr error, live bool) {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return
	}
	defer db.Close()

	// Both notes matter and neither supersedes the other: the scope note explains
	// the verdict, the error explains why the run ended.
	note := state.note
	if runErr != nil {
		if note != "" {
			note += "; "
		}
		note += runErr.Error()
	}
	if err := db.FinishRun(runID, state.verdict, state.outcome, state.needsVerification, note); err != nil && live {
		fmt.Printf("warning: could not record run completion: %v\n", err)
	}
}

// promoteConvergingResources flips this model's resources from `converging` to
// `clean` after a verified clean drift (the drift re-check confirming a pending
// apply). Best-effort: a missing catalog/resolver never fails the run.
//
// Both paths are model-scoped: the exact path matches the `tags.model` written by
// `ingest-manifest --model`, and the fallback matches this model's own resource
// definition names *and* excludes rows tagged to another model. The one case
// neither can separate is two models applying untagged manifests that declare a
// resource with the same identity name — those rows record no model at all. Tag
// manifests with `--model` to stay on the exact path.
func promoteConvergingResources(m *systemmodel.SystemModel, restricted bool) {
	if len(m.Desired) == 0 {
		return
	}
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return
	}
	defer db.Close()

	// Exact path: rows tagged with this model by `ingest-manifest --model <name>`.
	if !restricted {
		if n, err := db.PromoteConvergingToCleanByModel(m.Name); err == nil && n > 0 {
			return
		}
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
	_, _ = db.PromoteConvergingToClean(defs, m.Name)
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
	converge     bool
	only         []string
	dryRun       bool
	maxIters     int
	fromCatalog  bool
	catalogScope string

	onlySet     bool
	dryRunSet   bool
	maxItersSet bool
}

// validateRunFlags enforces the gate rules: convergence flags require --converge,
// and a catalog replay must name the records it replays. The first rule means a
// resource can't be named (or a plan dry-run requested) without explicitly opting
// into mutation. The second exists because there is no way to infer which
// already-ingested records belong to a model: records ingested by
// `pudl ingest-observe` carry whatever target their observer reported, so an
// unscoped replay would set-diff `desired` against every observation in the
// catalog — every model, every host, all time — and could report clean off
// another model's records.
func validateRunFlags(f runFlags) error {
	if f.fromCatalog && strings.TrimSpace(f.catalogScope) == "" {
		return fmt.Errorf("--from-catalog requires --catalog-scope (an observe snapshot ID, or the origin the records were ingested under)")
	}
	if !f.fromCatalog && strings.TrimSpace(f.catalogScope) != "" {
		return fmt.Errorf("--catalog-scope requires --from-catalog")
	}
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
	plan, err := acute.NewRunPlan(m, acute.RunRequest{
		Converge: f.converge,
		Only:     f.only,
		DryRun:   f.dryRun,
		MaxIters: f.maxIters,
	})
	if err != nil {
		return fmt.Sprintf("error: %v\n", err)
	}
	return renderRunPlan(plan)
}

func renderRunPlan(plan *acute.RunPlan) string {
	m := plan.Effective
	f := runFlags{
		converge: plan.Request.Converge,
		only:     plan.Request.Only,
		dryRun:   plan.Request.DryRun,
		maxIters: plan.Request.MaxIters,
	}
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
	runCmd.Flags().StringSliceVar(&runOnly, "only", nil, "converge only these resource selectors (requires --converge)")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "print the plan, execute nothing (requires --converge)")
	runCmd.Flags().IntVar(&runMaxIters, "max-iters", 5, "loop iteration cap (requires --converge)")
	runCmd.Flags().BoolVar(&runFromCatalog, "from-catalog", false, "drift over already-ingested records (inventory; no live observe); requires --catalog-scope")
	runCmd.Flags().StringVar(&runCatalogScope, "catalog-scope", "", "which already-ingested records --from-catalog replays: an observe snapshot ID, or the origin they were ingested under")
	runCmd.Flags().BoolVar(&runCheckUpstream, "check-upstream", false, "warn if any transitive upstream model (depends_on) is drifted/failed")
}

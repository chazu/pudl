package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

var (
	runMuRoot            string
	runConverge          bool
	runOnly              []string
	runDryRun            bool
	runMaxIters          int
	runMaxApplies        int
	runFromCatalog       bool
	runCatalogScope      string
	runCheckUpstream     bool
	runPopulateSpec      string
	runPopulateInput     []string
	runRequireApproval   bool
	runResumeID          string
	runApprovalStatus    string
	runMaxObservationAge time.Duration
)

var runMuRunnerFactory = func() muRunner { return execMu{} }

var runCmd = &cobra.Command{
	Use:   "run [<model>]",
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
	Args: func(cmd *cobra.Command, args []string) error {
		if runPopulateSpec != "" {
			if len(args) != 0 {
				return fmt.Errorf("--populate is an ad-hoc run and does not take a model name")
			}
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runError error) {
		if runResumeID == "" {
			// Cobra commands can be exercised repeatedly in one process by callers
			// and tests; approval state belongs only to a resumed invocation.
			runApprovalStatus = ""
		}
		if runPopulateSpec != "" && runConverge {
			return fmt.Errorf("--populate currently supports observe-only runs; scaffold a model before using --converge")
		}

		// The run's single catalog handle, borrowed by every phase that touches
		// the catalog. Opened lazily on first use, closed once here: this defer is
		// registered before the run-record finalizer below so it runs after it.
		cat := newRunCatalog(config.GetPudlDir())
		defer cat.Close()

		flags := runFlags{
			converge:     runConverge,
			only:         runOnly,
			dryRun:       runDryRun,
			maxIters:     runMaxIters,
			maxApplies:   runMaxApplies,
			fromCatalog:  runFromCatalog,
			catalogScope: runCatalogScope,
			// whether a convergence flag was explicitly set (for the gate rules)
			onlySet:       cmd.Flags().Changed("only"),
			dryRunSet:     cmd.Flags().Changed("dry-run"),
			maxItersSet:   cmd.Flags().Changed("max-iters"),
			maxAppliesSet: cmd.Flags().Changed("max-applies"),
		}
		if err := validateRunFlags(flags); err != nil {
			return err
		}

		// Resolve the model from the registered schemas (project .pudl/schema
		// wins over global ~/.pudl/schema). modelDir is where it was loaded from
		// — the base for eweSource + relative plugin paths.
		var name string
		var model *systemmodel.SystemModel
		var modelDir, pudlRoot string
		var bindingEvidence []wiring.BindingEvidence
		var sealedEvidence []wiring.SealedBindingEvidence
		var err error
		if runPopulateSpec != "" {
			model, modelDir, pudlRoot, err = adHocModel(runPopulateSpec, runPopulateInput)
		} else {
			name = args[0]
			template, templateDir, templateRoot, templateErr := resolveModelTemplate(name)
			if templateErr != nil {
				return templateErr
			}
			modelDir, pudlRoot = templateDir, templateRoot
			if !flags.dryRun {
				db, catalogErr := cat.required()
				if catalogErr != nil {
					return catalogErr
				}
				if reconcileErr := reconcileBindingDependencies(db, template); reconcileErr != nil {
					return fmt.Errorf("reconcile binding dependencies for %q: %w", template.Name, reconcileErr)
				}
			}
			if len(template.Bindings) == 0 {
				model, err = template.Elaborate(map[string]any{})
			} else {
				if cmd.Flags().Changed("max-observation-age") && runMaxObservationAge <= 0 {
					return fmt.Errorf("--max-observation-age must be greater than zero")
				}
				var db *database.CatalogDB
				var catalogErr error
				if flags.dryRun {
					db, catalogErr = cat.readOnlyRequired()
				} else {
					db, catalogErr = cat.required()
				}
				if catalogErr != nil {
					return catalogErr
				}
				schemas, schemaErr := inference.Shared(wsPolicy.SchemaSearchPaths...)
				if schemaErr != nil {
					return fmt.Errorf("load binding schemas: %w", schemaErr)
				}
				var maxAge *time.Duration
				if cmd.Flags().Changed("max-observation-age") {
					maxAge = &runMaxObservationAge
				}
				elaboration, resolveErr := (wiring.Resolver{Catalog: db, Schemas: schemas}).Elaborate(template, wiring.ResolveRequest{
					Workspace: effectiveWorkspaceName(), MaxObservationAge: maxAge,
					CurrentProducerRuns: currentRunSetProducerRuns(),
				})
				if resolveErr != nil {
					if jsonOutput && activeRunSet == nil {
						diagnostic := resolutionDiagnosticReport(template, flags, resolveErr)
						if rendered, renderErr := diagnostic.render(true); renderErr == nil {
							fmt.Print(rendered)
						}
					}
					return resolveErr
				}
				model = elaboration.Model
				bindingEvidence = elaboration.Evidence
			}
		}
		if err != nil {
			return err
		}
		model, sealedEvidence, err = resolveCurrentRunSetSealedModel(model)
		if err != nil {
			return fmt.Errorf("resolve sealed bindings for %q: %w", name, err)
		}
		name = model.Name
		if runRequireApproval && (!runConverge || runDryRun) {
			return fmt.Errorf("--require-approval requires --converge without --dry-run")
		}
		if runRequireApproval && !model.Convergent() {
			return fmt.Errorf("--require-approval requires a model with a converge arm")
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
		if reserved := currentRunSetMemberRunID(); reserved != "" {
			session.RunID = reserved
		}
		if runResumeID != "" {
			session.RunID = runResumeID
		}
		registerRunSetMemberRunID(session.RunID)
		if activeRunSet != nil {
			activeRunSet.lastModel = model
			activeRunSet.lastSealed = sealedEvidence
			activeRunSet.lastSnapshotID = session.SnapshotID
			activeRunSet.lastModelDir = modelDir
		}
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
		// finalizer below. `scoped` is set here rather than at the end because it
		// is known from the flags and must survive an early `return err` — a run
		// that died after applying under `--only` still must not look like an
		// unscoped one to the next run's budget calculation.
		finishState := &runFinishState{scoped: len(flags.only) > 0}
		approvalPending := false
		if !flags.dryRun {
			startRunRecord(cat, session.RunID, model.Name, mode, live)

			// The finalizer runs on every exit path, including an early `return err`,
			// so a run that ends badly is still recorded as *ended*. A row left
			// unfinished therefore means the process died without a word.
			defer func() {
				if !approvalPending {
					finishRunRecord(cat, session.RunID, *finishState, runError, live)
				}
			}()

			// A converge run can mutate before it is able to write a verdict, so the
			// model's previous verdict stops being trustworthy the moment it starts.
			// Clearing it to `unknown` up front means a crashed converge leaves
			// `unknown` rather than a stale `clean`. Observe-only runs change nothing,
			// so their model keeps its last real verdict.
			if mode == "converge" {
				persistRunStatus(cat, model.Name, "unknown", live)
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
			if err := recordModelInstance(cat, model, session.RunID); err != nil && live {
				fmt.Printf("warning: could not record model instance: %v\n", err)
			}

			// Reconcile this model's declared depends_on into model_depends_on facts
			// (add new edges, invalidate removed ones). Best-effort: a reconcile
			// failure must not fail the run. Warnings (e.g. unresolved deps) surface.
			if warns, err := reconcileModelDependencies(cat, model); err != nil {
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
			for _, w := range checkUpstreamFreshness(cat, model) {
				fmt.Printf("warning: %s\n", w)
			}
		}

		// muRoot is only needed by paths that run mu within an existing project
		// (plugin-observe live observe; differential drift). The ewe populate
		// path self-stages its own mu project, and --from-catalog runs no mu.
		// Best-effort: phases that genuinely need it validate when they run.
		muRoot := runMuRoot
		var removeAdHocMuRoot func()
		if muRoot == "" && !flags.fromCatalog {
			muRoot, _ = findMuRoot(modelDir)
			if muRoot == "" && runPopulateSpec != "" {
				muRoot, removeAdHocMuRoot, err = createAdHocMuRoot()
				if err != nil {
					return err
				}
				defer removeAdHocMuRoot()
			}
		}

		if flags.fromCatalog && len(model.Desired) == 0 {
			return fmt.Errorf("--from-catalog needs desired state; model %q declares none", model.Name)
		}

		// The subprocess seam. Passed explicitly rather than reached for, so the
		// whole run path can be driven by a scripted runner in a test — the
		// acceptance matrix the architecture report asks for needs no real mu.
		var mu muRunner = runMuRunnerFactory()

		report := &RunReport{
			ReportVersion: 1, RunSetID: currentRunSetID(), RunID: session.RunID,
			Model: model.Name, CompletionStatus: database.RunStatusRunning, OK: true,
			ApprovalStatus: runApprovalStatus, Bindings: bindingEvidence, SealedBindings: sealedEvidence,
		}
		reportPersisted := false
		defer func() {
			if flags.dryRun || reportPersisted {
				return
			}
			applyRunError(report, runError)
			persistRunReport(cat, report, live)
		}()

		if runRequireApproval {
			request, err := json.Marshal(approvalRequest{
				Model: model.Name, Only: flags.only, MaxIters: flags.maxIters,
				MaxApplies: flags.maxApplies,
			})
			if err != nil {
				return err
			}
			db, err := cat.required()
			if err != nil {
				return err
			}
			if err := db.SaveRunApproval(session.RunID, model.Name, request); err != nil {
				return err
			}
			report.Mode = "awaiting-approval"
			report.PendingApproval = true
			report.ApprovalStatus = "pending"
			approvalPending = true
			reportPersisted = persistRunReport(cat, report, live)
			out, err := report.render(jsonOutput)
			if err != nil {
				return err
			}
			if live && emitRunSetMemberOutput() {
				fmt.Print(out)
				fmt.Printf("approval pending: pudl run resume %s | pudl run reject %s\n", session.RunID, session.RunID)
			} else if emitRunSetMemberOutput() {
				fmt.Print(out)
			}
			return nil
		}
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
			budget := resolveApplyBudget(cat, model.Name, flags, live)
			cr, err := runConvergeLoop(cat, mu, effectiveModel, muRoot, modelDir, session.RunID, flags.maxIters, flags.dryRun, budget)
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
					pr, err := runPopulate(cat, mu, model, muRoot, modelDir, pudlRoot, session.RunID, session.SnapshotID)
					if err != nil {
						return err
					}
					report.Populate = pr
					scope = pr.SnapshotID
					if scope == "" {
						return fmt.Errorf("populate produced no snapshot to compare against")
					}
				}
				// This reads the records populate just wrote. Both borrow the run's
				// handle, so the read runs on the same connection as the write that
				// produced it — where the two phases used to open one apiece, this
				// was a second connection reading under the first one's writes.
				db, err := cat.required()
				if err != nil {
					return err
				}
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
				res, err := runDrift(cat, mu, model, muRoot, modelDir, session.RunID)
				if err != nil {
					return err
				}
				report.Drift = &res
			default:
				pr, err := runPopulate(cat, mu, model, muRoot, modelDir, pudlRoot, session.RunID, session.SnapshotID)
				if err != nil {
					return err
				}
				report.Populate = pr
			}
		}

		// Checks run on every arm, converge included. They used to sit inside the
		// observe-only branch, so a converge run reporting `clean` had evaluated
		// none of them and claimed more verification than it performed.
		//
		// A dry run is exempt: checks are read-only, but `runChecks` borrows the
		// run's catalog handle, and the handle is lazy precisely so a dry run never
		// creates data/sqlite/catalog.db. A dry run also writes no verdict, so
		// there would be nothing for a failed check to demote.
		//
		// Checks read the effective scoped model, not the original: invariant 2
		// requires one model shape across planning, execution, report scope,
		// promotion and scope-sensitive checks. Handing checks the unscoped
		// model would let a check assert over resources this run excluded.
		if len(effectiveModel.Checks) > 0 && !flags.dryRun {
			results, err := runChecks(cat, effectiveModel, modelDir, checkContext{
				runID:       session.RunID,
				fromCatalog: flags.fromCatalog,
				scope:       acute.NewTupleScope(model, effectiveModel),
			})
			if err != nil {
				return err
			}
			report.Checks = results
			if anyFailSeverityFailed(results) {
				report.OK = false
				// First error wins: a converge failure is what the operator needs to
				// see, and a check failure must not displace it.
				if runErr == nil {
					runErr = fmt.Errorf("one or more fail-severity checks did not pass")
				}
			}
		}

		applyRunError(report, runErr)
		if runErr == nil {
			report.CompletionStatus = database.RunStatusSucceeded
		}
		reportPersisted = persistRunReport(cat, report, live)
		out, err := report.render(jsonOutput)
		if err != nil {
			return err
		}
		if live && emitRunSetMemberOutput() {
			fmt.Print("\n")
		}
		if emitRunSetMemberOutput() {
			fmt.Print(out)
		}

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
		// A verdict demoted by a check reads as ordinary resource drift on the model
		// row, so say which checks did it — otherwise an operator hunts for drift
		// that is not there.
		if names := failedFailSeverityNames(report.Checks); len(names) > 0 && verdict == "drifted" {
			finishState.addNote(fmt.Sprintf("verdict demoted to %q by fail-severity check(s): %s",
				verdict, strings.Join(names, ", ")))
			if live {
				fmt.Printf("\nnote: fail-severity check(s) did not pass: %s\n", strings.Join(names, ", "))
				fmt.Println("      the model's resources may match desired state; the failure is the check's assertion")
			}
		}

		restricted := len(flags.only) > 0
		rowVerdict := modelRowVerdict(verdict, restricted)
		if rowVerdict != verdict {
			note := fmt.Sprintf("verdict %q covers only the --only scope (%s); model status left %q",
				verdict, strings.Join(flags.only, ","), rowVerdict)
			finishState.addNote(note)
			if live {
				fmt.Printf("\nnote: %s\n", note)
				fmt.Println("      a scoped ∅ does not prove the whole model clean; re-run unscoped to establish it")
			}
		}
		persistRunStatus(cat, model.Name, rowVerdict, live)

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
			promoteConvergingResources(cat, effectiveModel, restricted)
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
	verdict := phaseVerdict(r, f)
	// A fail-severity check that did not pass says the model is not in the state
	// it declares, so a `clean` written over it would claim verification the run
	// contradicts. Only `clean` is demoted: `drifted` and `failed` are already at
	// least as severe, `unknown` means the run could not prove the state at all
	// (and a check over a catalog possibly missing this run's receipt cannot turn
	// that ignorance into knowledge), and "" writes nothing by design.
	//
	// `drifted` rather than `failed`: the run's machinery worked — the apply
	// succeeded, the re-observation completed. What failed is an assertion about
	// the resulting state, which is what `drifted` names. `failed` would invite a
	// manual re-apply, the same mistake D2 rejected for lost receipts.
	if verdict == "clean" && anyFailSeverityFailed(r.Checks) {
		return "drifted"
	}
	return verdict
}

// phaseVerdict is the verdict the run's phases alone support, before checks are
// allowed to demote it.
func phaseVerdict(r *RunReport, f runFlags) string {
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
func persistRunStatus(cat *runCatalog, name, status string, live bool) {
	if status == "" {
		return
	}
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to record status %q: %v\n", status, err)
		}
		return
	}
	if err := db.UpdateStatus(modelTargetKey(name), status); err != nil && live {
		fmt.Printf("warning: could not record status %q: %v\n", status, err)
	}
}

// resolveApplyBudget works out how many applies this run may make, from how many
// the model has already spent since it was last verified clean.
//
// The per-run `--max-iters` cap gives a halting guarantee inside one process and
// none at all across processes: a scheduler on `freshness.every`, or a
// crash-loop supervisor, grants a fresh cap on every restart, so a model that
// oscillates applies without bound. This is the durable half.
//
// Returns nil — no constraint, previous behaviour exactly — when the budget is
// disabled (`--max-applies 0`), when the run does not converge, or when the
// history cannot be read. A catalog that cannot answer must not silently refuse
// to apply; that failure mode is worse than the one being prevented.
func resolveApplyBudget(cat *runCatalog, model string, flags runFlags, live bool) *int {
	if !flags.converge || flags.dryRun || flags.maxApplies <= 0 {
		return nil
	}
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to read this model's apply history: %v\n", err)
			fmt.Println("         the durable apply budget is not enforced for this run")
		}
		return nil
	}
	spent, err := db.AppliesSinceLastClean(model)
	if err != nil {
		if live {
			fmt.Printf("warning: could not read this model's apply history: %v\n", err)
			fmt.Println("         the durable apply budget is not enforced for this run")
		}
		return nil
	}

	remaining := flags.maxApplies - spent
	if remaining < 0 {
		remaining = 0
	}
	if live && spent > 0 {
		fmt.Printf("apply budget: %d of %d remaining (spent since this model was last verified clean)\n",
			remaining, flags.maxApplies)
	}
	return &remaining
}

// runFinishState is what a completed run concluded, handed to the deferred
// finalizer so every exit path — including an early `return err` — records a
// terminal run row.
type runFinishState struct {
	verdict           string
	outcome           string
	needsVerification bool
	// note explains a verdict whose meaning is not evident from the value alone —
	// a scoped run whose model row diverges from its run row, or a verdict a
	// failed check demoted. More than one can apply, so they accumulate.
	note string
	// scoped records that `--only` restricted this run. A scoped `clean` covers a
	// subset, so it must not reset the model's durable apply budget.
	scoped bool
}

// addNote appends an explanation, keeping any already recorded. Two notes can
// both be true of one run (a scoped run whose check also failed), and dropping
// either would leave a verdict half-explained.
func (s *runFinishState) addNote(note string) {
	if note == "" {
		return
	}
	if s.note != "" {
		s.note += "; "
	}
	s.note += note
}

// startRunRecord opens the run's audit row before any phase runs, and reports any
// earlier run of this model that never finished. An unfinished row means a prior
// invocation died without recording a verdict, so the status that model currently
// carries predates it. Best-effort: auditing must not fail the run.
func startRunRecord(cat *runCatalog, runID, model, mode string, live bool) {
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to record the run: %v\n", err)
		}
		return
	}

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
func finishRunRecord(cat *runCatalog, runID string, state runFinishState, runErr error, live bool) {
	// An open failure goes unreported here alone: startRunRecord borrowed the same
	// handle and already said so, and there is no row to finish anyway.
	db, err := cat.optional()
	if err != nil {
		return
	}

	// Both notes matter and neither supersedes the other: the scope note explains
	// the verdict, the error explains why the run ended.
	note := state.note
	if runErr != nil {
		if note != "" {
			note += "; "
		}
		note += runErr.Error()
	}
	completionStatus := database.RunStatusSucceeded
	if runErr != nil {
		completionStatus = database.RunStatusFailed
	}
	if err := db.FinishRun(runID, database.RunConclusion{
		CompletionStatus:  completionStatus,
		Verdict:           state.verdict,
		Outcome:           state.outcome,
		NeedsVerification: state.needsVerification,
		Note:              note,
		Scoped:            state.scoped,
	}); err != nil && live {
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
func promoteConvergingResources(cat *runCatalog, m *systemmodel.SystemModel, restricted bool) {
	if len(m.Desired) == 0 {
		return
	}
	db, err := cat.optional()
	if err != nil {
		return
	}

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
	return len(failedFailSeverityNames(results)) > 0
}

// failedFailSeverityNames lists the fail-severity checks that did not pass.
// Passed is already the gating verdict — under `--only` a check whose only
// matches were out of scope passes — so the exit code, the rendered verdict and
// the recorded reason all follow from the same field.
func failedFailSeverityNames(results []CheckResult) []string {
	var names []string
	for _, c := range results {
		if !c.Passed && c.Severity == "fail" {
			names = append(names, c.Name)
		}
	}
	return names
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
	converge bool
	only     []string
	dryRun   bool
	maxIters int
	// maxApplies is the durable cap: the applies this model may make in total
	// since it was last verified clean, across runs. 0 disables it.
	maxApplies   int
	fromCatalog  bool
	catalogScope string

	onlySet       bool
	dryRunSet     bool
	maxItersSet   bool
	maxAppliesSet bool
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
		if f.maxApplies < 0 {
			return fmt.Errorf("--max-applies must be >= 0 (0 disables the durable apply budget)")
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
	case f.maxAppliesSet:
		return fmt.Errorf("--max-applies requires --converge")
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
	runCmd.Flags().StringVar(&runPopulateSpec, "populate", "", "Run an unregistered observer as plugin:<name>")
	runCmd.Flags().StringArrayVar(&runPopulateInput, "input", nil, "Ad-hoc populate input key=value (repeatable)")
	runCmd.Flags().StringVar(&runMuRoot, "mu-root", "", "mu project root to run within (default: discover mu.cue from the model dir)")
	runCmd.Flags().BoolVar(&runConverge, "converge", false, "opt into the convergence loop (mutates the target)")
	runCmd.Flags().StringSliceVar(&runOnly, "only", nil, "converge only these resource selectors (requires --converge)")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "print the plan, execute nothing (requires --converge)")
	runCmd.Flags().IntVar(&runMaxIters, "max-iters", 5, "loop iteration cap (requires --converge)")
	runCmd.Flags().IntVar(&runMaxApplies, "max-applies", 20, "durable cap on applies since this model was last verified clean; 0 disables (requires --converge)")
	runCmd.Flags().BoolVar(&runFromCatalog, "from-catalog", false, "drift over already-ingested records (inventory; no live observe); requires --catalog-scope")
	runCmd.Flags().StringVar(&runCatalogScope, "catalog-scope", "", "which already-ingested records --from-catalog replays: an observe snapshot ID, or the origin they were ingested under")
	runCmd.Flags().BoolVar(&runCheckUpstream, "check-upstream", false, "warn if any transitive upstream model (depends_on) is drifted/failed")
	runCmd.Flags().DurationVar(&runMaxObservationAge, "max-observation-age", 0, "reject a bound producer snapshot older than this duration")
	runCmd.Flags().BoolVar(&runRequireApproval, "require-approval", false, "persist the converge request and wait for `pudl run resume <run-id>`")
}

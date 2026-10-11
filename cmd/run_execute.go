package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

// resolvedRun is a model ready to run: elaborated, sealed-resolved, and located.
type resolvedRun struct {
	model           *systemmodel.SystemModel
	modelDir        string
	pudlRoot        string
	bindingEvidence []wiring.BindingEvidence
	sealedEvidence  []wiring.SealedBindingEvidence
}

// executeRun drives one model through the ACUTE cycle. It is the whole of
// `pudl run`, shared by standalone runs, run-set members and approval resumes;
// each caller builds its own runOptions rather than re-entering another
// command's handler.
//
// The returned report is nil when the run failed before it had one.
//
// ctx is the invocation's lifetime. Cancelling it (the first Ctrl-C) stops the
// mu subprocess deps.mu is bound to, and the run then concludes `cancelled`
// through its ordinary exit path — with needs-verification if an apply may have
// been in flight. There is no resume: an interrupted run is simply re-run.
func executeRun(ctx context.Context, opts runOptions, deps runDeps) (finalReport *RunReport, runError error) {
	emitRunProgress("prepare", opts.model, "started")
	defer func() {
		state := "completed"
		if runError != nil {
			state = "failed"
		}
		emitRunProgress("run", opts.model, state)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	approvalStatus := ""
	if opts.resumeID != "" {
		// Approval state belongs only to a resumed invocation.
		approvalStatus = opts.approvalStatus
	}
	if opts.populateSpec != "" && opts.converge {
		return nil, fmt.Errorf("--populate currently supports observe-only runs; scaffold a model before using --converge")
	}

	// The run's single catalog handle, borrowed by every phase that touches
	// the catalog. Opened lazily on first use, closed once here: this defer is
	// registered before the run-record finalizer below so it runs after it.
	cat := newRunCatalog(effectivePudlDir())
	defer cat.Close()

	flags := opts.runFlags
	if err := validateRunFlags(flags); err != nil {
		return nil, err
	}

	resolved, err := resolveRunModel(ctx, cat, opts, deps)
	if err != nil {
		return nil, err
	}
	model, modelDir, pudlRoot := resolved.model, resolved.modelDir, resolved.pudlRoot
	if opts.requireApproval && (!opts.converge || opts.dryRun) {
		return nil, fmt.Errorf("--require-approval requires --converge without --dry-run")
	}
	if opts.requireApproval && !model.Convergent() {
		return nil, fmt.Errorf("--require-approval requires a model with a converge arm")
	}
	plan, err := acute.NewRunPlan(model, acute.RunRequest{
		Converge:    flags.converge,
		Only:        flags.only,
		DryRun:      flags.dryRun,
		MaxIters:    flags.maxIters,
		FromCatalog: flags.fromCatalog,
	})
	if err != nil {
		return nil, err
	}
	session := acute.NewRunSession(plan)
	if reserved := deps.set.memberRunID(); reserved != "" {
		session.RunID = reserved
	}
	if opts.resumeID != "" {
		session.RunID = opts.resumeID
	}
	deps.set.registerMemberRunID(session.RunID)
	deps.set.retainMember(model, resolved.sealedEvidence, session.SnapshotID, modelDir)
	effectiveModel := session.Plan.Effective

	live := !jsonOutput
	if live {
		fmt.Fprint(outw(), renderRunPlan(plan))
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

	recordRunContext(cat, model, session.RunID, opts, live)

	// muRoot is only needed by paths that run mu within an existing project
	// (plugin-observe live observe; differential drift). The ewe populate
	// path self-stages its own mu project, and --from-catalog runs no mu.
	// Best-effort: phases that genuinely need it validate when they run.
	muRoot := opts.muRoot
	if muRoot == "" && !flags.fromCatalog {
		muRoot, _ = findMuRoot(modelDir)
		if muRoot == "" && opts.populateSpec != "" && model.Populate.Kind() == systemmodel.KindPluginObserve {
			var removeAdHocMuRoot func()
			muRoot, removeAdHocMuRoot, err = createAdHocMuRoot()
			if err != nil {
				return nil, err
			}
			defer removeAdHocMuRoot()
		}
	}

	if flags.fromCatalog && len(model.Desired) == 0 {
		return nil, fmt.Errorf("--from-catalog needs desired state; model %q declares none", model.Name)
	}

	report := &RunReport{
		ResourceScope: append([]string(nil), flags.only...),
		ReportVersion: 2, RunSetID: deps.set.id(), RunID: session.RunID,
		Model: model.Name, CompletionStatus: database.RunStatusRunning, OK: true,
		ApprovalStatus: approvalStatus, Bindings: resolved.bindingEvidence, SealedBindings: resolved.sealedEvidence,
	}
	reportPersisted := false
	defer func() {
		if flags.dryRun || reportPersisted {
			return
		}
		applyRunError(report, runError)
		persistRunReport(cat, report, live)
	}()

	if opts.requireApproval {
		request, err := json.Marshal(newApprovalRequest(model.Name, flags, muRoot))
		if err != nil {
			return report, err
		}
		db, err := cat.required()
		if err != nil {
			return report, err
		}
		if err := db.SaveRunApproval(session.RunID, model.Name, request); err != nil {
			return report, err
		}
		report.Mode = "awaiting-approval"
		report.PendingApproval = true
		report.ApprovalStatus = "pending"
		approvalPending = true
		reportPersisted = persistRunReport(cat, report, live)
		out, err := report.render(jsonOutput)
		if err != nil {
			return report, err
		}
		if live && deps.set.emitOutput() {
			fmt.Fprint(outw(), out)
			fmt.Fprintf(outw(), "approval pending: pudl run resume %s | pudl run reject %s\n", session.RunID, session.RunID)
		} else if deps.set.emitOutput() {
			fmt.Fprint(outw(), out)
		}
		return report, nil
	}

	phases := runPhaseInput{
		ctx: ctx,
		cat: cat, mu: deps.mu, model: model, effective: effectiveModel, flags: flags,
		muRoot: muRoot, modelDir: modelDir, pudlRoot: pudlRoot, session: session, live: live,
	}
	// An interrupt that arrived while the run was being prepared stops it before
	// any phase starts; the deferred finalizers record it as cancelled.
	if err := ctx.Err(); err != nil {
		return report, fmt.Errorf("run interrupted before its phases began: %w", err)
	}
	runErr, err := executeRunPhases(phases, report)
	if err != nil {
		return report, err
	}
	reportPersisted, err = concludeRun(phases, report, finishState, runErr, deps)
	return report, err
}

// resolveRunModel loads the model the options name — or builds the ad-hoc one
// --populate describes — elaborates its bindings, and resolves sealed sources.
func resolveRunModel(ctx context.Context, cat *runCatalog, opts runOptions, deps runDeps) (*resolvedRun, error) {
	resolved := &resolvedRun{}
	var err error
	name := opts.model
	if opts.populateSpec != "" {
		resolved.model, resolved.modelDir, resolved.pudlRoot, err = adHocModel(ctx, opts.populateSpec, opts.populateInput)
	} else {
		resolved.model, err = elaborateRunTemplate(cat, opts, deps, resolved)
	}
	if err != nil {
		return nil, err
	}
	resolved.model, resolved.sealedEvidence, err = deps.set.resolveSealedModel(resolved.model)
	if err != nil {
		return nil, fmt.Errorf("resolve sealed bindings for %q: %w", name, err)
	}
	return resolved, nil
}

// elaborateRunTemplate resolves a registered model template (project
// .pudl/schema wins over global ~/.pudl/schema) and elaborates its bindings.
// The template's directory is the base for eweSource and relative plugin paths.
func elaborateRunTemplate(cat *runCatalog, opts runOptions, deps runDeps, resolved *resolvedRun) (*systemmodel.SystemModel, error) {
	template, templateDir, templateRoot, err := resolveModelTemplate(opts.model)
	if err != nil {
		return nil, err
	}
	resolved.modelDir, resolved.pudlRoot = templateDir, templateRoot
	if !opts.dryRun {
		db, catalogErr := cat.required()
		if catalogErr != nil {
			return nil, catalogErr
		}
		if reconcileErr := reconcileBindingDependencies(db, template); reconcileErr != nil {
			return nil, fmt.Errorf("reconcile binding dependencies for %q: %w", template.Name, reconcileErr)
		}
	}
	if len(template.Bindings) == 0 {
		return template.Elaborate(map[string]any{})
	}
	if opts.maxObservationAgeSet && opts.maxObservationAge <= 0 {
		return nil, fmt.Errorf("--max-observation-age must be greater than zero")
	}
	var db *database.CatalogDB
	if opts.dryRun {
		db, err = cat.readOnlyRequired()
	} else {
		db, err = cat.required()
	}
	if err != nil {
		return nil, err
	}
	schemas, err := inference.Shared(wsPolicy.SchemaSearchPaths...)
	if err != nil {
		return nil, fmt.Errorf("load binding schemas: %w", err)
	}
	var maxAge *time.Duration
	if opts.maxObservationAgeSet {
		age := opts.maxObservationAge
		maxAge = &age
	}
	elaboration, err := (wiring.Resolver{Catalog: db, Schemas: schemas}).Elaborate(template, wiring.ResolveRequest{
		Workspace: effectiveWorkspaceName(), MaxObservationAge: maxAge,
		CurrentProducerRuns: deps.set.producerRuns(),
	})
	if err != nil {
		if jsonOutput && deps.set == nil {
			diagnostic := resolutionDiagnosticReport(template, opts.runFlags, err)
			if rendered, renderErr := diagnostic.render(true); renderErr == nil {
				fmt.Fprint(outw(), rendered)
			}
		}
		return nil, err
	}
	resolved.bindingEvidence = elaboration.Evidence
	return elaboration.Model, nil
}

// recordRunContext writes the model instance and its dependency facts, and
// runs the opt-in upstream freshness guard.
//
// A dry run must not mutate catalog state, memberships, facts or statuses:
// it is documented as showing what *would* happen. Both writes are real
// mutations, and both used to run unconditionally — before the dry-run branch
// was ever reached — so `--dry-run` created snapshot entries, item entries,
// collection memberships, raw files and model_depends_on facts.
func recordRunContext(cat *runCatalog, model *systemmodel.SystemModel, runID string, opts runOptions, live bool) {
	if !opts.dryRun {
		// Record the instance in the catalog (identity = name) so every model
		// that's been run is inventoriable via `pudl list`/`query`. Best-effort:
		// a recording failure must not fail the run.
		if err := recordModelInstance(cat, model, runID); err != nil && live {
			fmt.Fprintf(errw(), "warning: could not record model instance: %v\n", err)
		}

		// Reconcile this model's declared depends_on into model_depends_on facts
		// (add new edges, invalidate removed ones). Best-effort: a reconcile
		// failure must not fail the run. Warnings (e.g. unresolved deps) surface.
		if warns, err := reconcileModelDependencies(cat, model); err != nil {
			if live {
				fmt.Fprintf(errw(), "warning: could not reconcile dependencies: %v\n", err)
			}
		} else if live {
			for _, w := range warns {
				fmt.Fprintf(errw(), "warning: %s\n", w)
			}
		}
	} else if live {
		fmt.Fprintln(outw(), "dry-run: skipping model-instance and dependency-fact writes")
	}

	// Opt-in stale-input guard: warn if any transitive upstream is drifted/failed.
	if opts.checkUpstream && live {
		for _, w := range checkUpstreamFreshness(cat, model) {
			fmt.Fprintf(errw(), "warning: %s\n", w)
		}
	}
}

// runPhaseInput is what the phase dispatch and the conclusion share.
type runPhaseInput struct {
	ctx       context.Context
	cat       *runCatalog
	mu        muRunner
	model     *systemmodel.SystemModel
	effective *systemmodel.SystemModel
	flags     runFlags
	muRoot    string
	modelDir  string
	pudlRoot  string
	session   *acute.RunSession
	live      bool
}

// executeRunPhases runs the converge loop or the observe-only arm. runErr is a
// run outcome recorded on the report (a converge failure); err aborts the run.
func executeRunPhases(in runPhaseInput, report *RunReport) (runErr error, err error) {
	emitRunProgress("observe", in.model.Name, "started")
	if in.ctx == nil {
		in.ctx = runOperationContext(in.mu)
	}
	model, flags := in.model, in.flags
	switch {
	case flags.converge && model.Convergent():
		report.Mode = "converge"
		if flags.dryRun {
			report.Mode = "dry-run"
		}
		if in.live {
			fmt.Fprintln(outw(), "\n— converge —")
		}
		budget := resolveApplyBudget(in.cat, model.Name, flags, in.live)
		cr, convergeErr := runConvergeLoop(in.cat, in.mu, in.effective, in.muRoot, in.modelDir, in.session.RunID, flags.maxIters, flags.dryRun, budget)
		report.Converge = cr
		if convergeErr != nil {
			report.OK = false
			runErr = convergeErr
		}
		return runErr, nil
	}

	report.Mode = "observe-only"
	// A model with `desired` flags drift; without it, populate.
	switch {
	case len(model.Desired) > 0 && useInventoryDrift(model, flags.fromCatalog):
		// Inventory: set-diff desired vs already-ingested catalog records
		// (no live observe). Auto-selected for inventory observers
		// (EweTarget, or #PluginObserve differential:false); --from-catalog
		// forces it for any model.
		report.Mode = "observe-only (inventory)"
		identity, namespace, err := inventoryIdentityPolicy()
		if err != nil {
			return nil, err
		}
		// Scope is mandatory on both arms: a live run compares against the
		// snapshot it just populated, and a replay compares against the
		// scope the operator named (validateRunFlags requires one). An
		// empty scope would query every observe record in the catalog.
		var scope string
		if flags.fromCatalog {
			scope = strings.TrimSpace(flags.catalogScope)
		} else {
			pr, err := runPopulate(in.cat, in.mu, model, in.muRoot, in.modelDir, in.pudlRoot, in.session.RunID, in.session.SnapshotID)
			if err != nil {
				return nil, err
			}
			report.Populate = pr
			scope = pr.SnapshotID
			if scope == "" {
				return nil, fmt.Errorf("populate produced no snapshot to compare against")
			}
		}
		// This reads the records populate just wrote. Both borrow the run's
		// handle, so the read runs on the same connection as the write that
		// produced it — where the two phases used to open one apiece, this
		// was a second connection reading under the first one's writes.
		db, err := in.cat.required()
		if err != nil {
			return nil, err
		}
		res, err := runInventoryDriftContext(in.ctx, db, scope, model.Desired, identity, namespace)
		if err != nil {
			return nil, err
		}
		// A replay is not an observation of the live system, so its verdict
		// cannot promote resources or write a clean status.
		res.Verified = !flags.fromCatalog && !res.Uncertain
		report.Drift = &res
		if res.Uncertain {
			return fmt.Errorf("incomplete inventory cannot establish absence; collect and select an explicitly complete snapshot"), nil
		}
	case len(model.Desired) > 0:
		// Differential: live observe with desired-as-sources (k8s-style).
		res, err := runDrift(in.cat, in.mu, model, in.muRoot, in.modelDir, in.session.RunID)
		if err != nil {
			return nil, err
		}
		report.Drift = &res
	case model.Populate.Kind() == systemmodel.KindNone:
		// Checks-only: evaluate checks over what the catalog already holds.
		report.Mode = "checks-only"
	default:
		pr, err := runPopulate(in.cat, in.mu, model, in.muRoot, in.modelDir, in.pudlRoot, in.session.RunID, in.session.SnapshotID)
		if err != nil {
			return nil, err
		}
		report.Populate = pr
	}
	return nil, nil
}

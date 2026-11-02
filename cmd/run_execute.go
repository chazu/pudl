package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

// resolvedRun is a model ready to run: decoded and located.
type resolvedRun struct {
	model    *systemmodel.SystemModel
	modelDir string
}

// executeRun drives one member of a check through the observe-only cycle.
//
// The returned report is nil when the run failed before it had one.
//
// ctx is the invocation's lifetime. Cancelling it (the first Ctrl-C) stops the
// subprocess the run is waiting on, and the run then concludes `cancelled`
// through its ordinary exit path. An interrupted run is simply re-run.
func executeRun(ctx context.Context, opts runOptions, op *checkOperation) (finalReport *RunReport, runError error) {
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

	// The run's single catalog handle, borrowed by every phase that touches
	// the catalog. Opened lazily on first use, closed once here: this defer is
	// registered before the run-record finalizer below so it runs after it.
	cat := newRunCatalog(effectivePudlDir())
	defer cat.Close()

	flags := opts.runFlags
	if err := validateRunFlags(flags); err != nil {
		return nil, err
	}

	resolved, err := resolveRunModel(opts)
	if err != nil {
		return nil, err
	}
	model, modelDir := resolved.model, resolved.modelDir
	session := acute.NewRunSession()
	if op.nextRunID != "" {
		session.RunID = op.nextRunID
	}
	op.lastRunID = session.RunID

	live := !jsonOutput
	if live {
		fmt.Fprint(outw(), renderRunPlan(model))
	}

	// finishState is populated once the run concludes and read by the deferred
	// finalizer below.
	finishState := &runFinishState{}
	startRunRecord(cat, session.RunID, model.Name, "observe-only", live)

	// The finalizer runs on every exit path, including an early `return err`,
	// so a run that ends badly is still recorded as *ended*. A row left
	// unfinished therefore means the process died without a word.
	defer func() {
		finishRunRecord(cat, session.RunID, *finishState, runError, live)
	}()

	recordRunContext(cat, model, session.RunID, opts, live)

	if flags.fromCatalog && len(model.Desired) == 0 {
		return nil, fmt.Errorf("--from-catalog needs desired state; model %q declares none", model.Name)
	}

	report := &RunReport{
		ReportVersion: 2, RunSetID: op.id, RunID: session.RunID,
		Model: model.Name, CompletionStatus: database.RunStatusRunning, OK: true,
	}
	reportPersisted := false
	defer func() {
		if reportPersisted {
			return
		}
		applyRunError(report, runError)
		persistRunReport(cat, report, live)
	}()

	phases := runPhaseInput{
		ctx: ctx,
		cat: cat, model: model, flags: flags,
		modelDir: modelDir, session: session, live: live,
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
	reportPersisted, err = concludeRun(phases, report, finishState, runErr)
	return report, err
}

// resolveRunModel loads the registered model the options name (project
// .pudl/schema wins over global ~/.pudl/schema) — or builds the ad-hoc one
// --populate describes. The model's directory is the base for relative
// command paths.
func resolveRunModel(opts runOptions) (*resolvedRun, error) {
	if opts.populateSpec != "" {
		model, modelDir, err := adHocModel(opts.populateSpec)
		if err != nil {
			return nil, err
		}
		return &resolvedRun{model: model, modelDir: modelDir}, nil
	}
	template, templateDir, _, err := resolveModelTemplate(opts.model)
	if err != nil {
		return nil, err
	}
	model, err := template.Model()
	if err != nil {
		return nil, err
	}
	return &resolvedRun{model: model, modelDir: templateDir}, nil
}

// recordRunContext writes the model instance and its dependency facts, and
// runs the opt-in upstream freshness guard.
func recordRunContext(cat *runCatalog, model *systemmodel.SystemModel, runID string, opts runOptions, live bool) {
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

	// Opt-in stale-input guard: warn if any transitive upstream is drifted/failed.
	if opts.checkUpstream && live {
		for _, w := range checkUpstreamFreshness(cat, model) {
			fmt.Fprintf(errw(), "warning: %s\n", w)
		}
	}
}

// runPhaseInput is what the phase dispatch and the conclusion share.
type runPhaseInput struct {
	ctx      context.Context
	cat      *runCatalog
	model    *systemmodel.SystemModel
	flags    runFlags
	modelDir string
	session  *acute.RunSession
	live     bool
}

// executeRunPhases runs the observe-only arm. runErr is a run outcome recorded
// on the report (incomplete inventory); err aborts the run.
func executeRunPhases(in runPhaseInput, report *RunReport) (runErr error, err error) {
	emitRunProgress("observe", in.model.Name, "started")
	if in.ctx == nil {
		in.ctx = context.Background()
	}
	model, flags := in.model, in.flags
	report.Mode = "observe-only"
	// A model with `desired` flags drift; without it, populate.
	switch {
	case len(model.Desired) > 0:
		// Inventory: populate, then set-diff desired against the records it
		// produced; --from-catalog replays already-ingested records instead.
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
			pr, err := runPopulate(in.ctx, in.cat, model, in.modelDir, in.session.RunID, in.session.SnapshotID)
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
		// cannot write a clean status.
		res.Verified = !flags.fromCatalog && !res.Uncertain
		report.Drift = &res
		if res.Uncertain {
			return fmt.Errorf("incomplete inventory cannot establish absence; collect and select an explicitly complete snapshot"), nil
		}
	case model.Populate.Kind() == systemmodel.KindNone:
		// Checks-only: evaluate checks over what the catalog already holds.
		report.Mode = "checks-only"
	default:
		pr, err := runPopulate(in.ctx, in.cat, model, in.modelDir, in.session.RunID, in.session.SnapshotID)
		if err != nil {
			return nil, err
		}
		report.Populate = pr
	}
	return nil, nil
}

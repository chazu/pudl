package operation

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inventory"
	"github.com/chazu/pudl/internal/systemmodel"
)

// modelRun is one model's observe-only run within a check: what every phase
// and the conclusion share.
type modelRun struct {
	ctx      context.Context
	ws       Workspace
	out      Output
	opts     Options
	cat      *runCatalog
	model    *systemmodel.SystemModel
	modelDir string
	session  *acute.RunSession
}

// runModel drives one member of a check through the observe-only cycle.
//
// The returned report is nil when the run failed before it had one.
//
// c.ctx is the invocation's lifetime. Cancelling it (the first Ctrl-C) stops
// the subprocess the run is waiting on, and the run then concludes `cancelled`
// through its ordinary exit path. An interrupted run is simply re-run.
func (c *check) runModel(name string) (finalReport *checkreport.ModelReport, runError error) {
	c.out.progress("prepare", name, "started")
	defer func() {
		state := "completed"
		if runError != nil {
			state = "failed"
		}
		c.out.progress("run", name, state)
	}()
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// The run's single catalog handle, borrowed by every phase that touches
	// the catalog. Opened lazily on first use, closed once here: this defer is
	// registered before the run-record finalizer below so it runs after it.
	cat := newRunCatalog(c.ws.PudlDir)
	defer cat.Close()

	if err := ValidateOptions(c.opts); err != nil {
		return nil, err
	}

	model, modelDir, err := c.resolveModel(name)
	if err != nil {
		return nil, err
	}
	session := acute.NewRunSession()
	if c.nextRunID != "" {
		session.RunID = c.nextRunID
	}
	c.lastRunID = session.RunID
	run := &modelRun{
		ctx: ctx, ws: c.ws, out: c.out, opts: c.opts,
		cat: cat, model: model, modelDir: modelDir, session: session,
	}

	if c.out.Live {
		fmt.Fprint(c.out.Out, checkreport.RenderPlan(model))
	}

	// finishState is populated once the run concludes and read by the deferred
	// finalizer below.
	finishState := &runFinishState{}
	startRunRecord(cat, c.out, session.RunID, model.Name, "observe-only")

	// The finalizer runs on every exit path, including an early `return err`,
	// so a run that ends badly is still recorded as *ended*. A row left
	// unfinished therefore means the process died without a word.
	defer func() {
		finishRunRecord(cat, c.out, session.RunID, *finishState, runError)
	}()

	run.recordContext()

	if c.opts.FromCatalog && len(model.Desired) == 0 {
		return nil, fmt.Errorf("--from-catalog needs desired state; model %q declares none", model.Name)
	}

	report := &checkreport.ModelReport{
		ReportVersion: checkreport.ModelReportVersion, RunSetID: c.id, RunID: session.RunID,
		Model: model.Name, CompletionStatus: database.RunStatusRunning, OK: true,
	}
	reportPersisted := false
	defer func() {
		if reportPersisted {
			return
		}
		applyRunError(report, runError)
		persistRunReport(cat, c.out, report)
	}()

	// An interrupt that arrived while the run was being prepared stops it before
	// any phase starts; the deferred finalizers record it as cancelled.
	if err := ctx.Err(); err != nil {
		return report, fmt.Errorf("run interrupted before its phases began: %w", err)
	}
	runErr, err := run.phases(report)
	if err != nil {
		return report, err
	}
	reportPersisted, err = run.conclude(report, finishState, runErr)
	return report, err
}

// resolveModel loads the registered model name names (the first model search
// dir that has it wins) — or builds the ad-hoc one --populate describes — and
// checks its schema contracts before any command runs. The model's directory
// is the base for relative command paths; an ad-hoc model's is the current
// directory.
func (c *check) resolveModel(name string) (*systemmodel.SystemModel, string, error) {
	model, modelDir, err := c.loadModel(name)
	if err != nil {
		return nil, "", err
	}
	if err := c.ws.CheckModelSchemas(model); err != nil {
		return nil, "", err
	}
	return model, modelDir, nil
}

func (c *check) loadModel(name string) (*systemmodel.SystemModel, string, error) {
	if c.opts.PopulateSpec != "" {
		model, err := systemmodel.AdHocModel(c.opts.PopulateSpec)
		if err != nil {
			return nil, "", err
		}
		return model, currentDir(), nil
	}
	template, err := systemmodel.ResolveTemplate(c.ws.ModelDirs, name)
	if err != nil {
		return nil, "", err
	}
	model, err := template.Model()
	if err != nil {
		return nil, "", err
	}
	return model, template.Origin.LoadDir, nil
}

func currentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// recordContext writes the model instance and its dependency facts, and runs
// the opt-in upstream freshness guard.
func (r *modelRun) recordContext() {
	// Record the instance in the catalog (identity = name) so every model
	// that's been checked is inventoriable via `pudl list`/`query`.
	// Best-effort: a recording failure must not fail the run.
	if err := r.recordModelInstance(); err != nil {
		r.out.warnf("warning: could not record model instance: %v\n", err)
	}

	// Reconcile this model's declared depends_on into model_depends_on facts
	// (add new edges, invalidate removed ones). Best-effort: a reconcile
	// failure must not fail the run. Warnings (e.g. unresolved deps) surface.
	if warns, err := reconcileModelDependencies(r.cat, r.ws.ModelDirs, r.model); err != nil {
		r.out.warnf("warning: could not reconcile dependencies: %v\n", err)
	} else {
		for _, w := range warns {
			r.out.warnf("warning: %s\n", w)
		}
	}

	// Opt-in stale-input guard: warn if any transitive upstream is drifted/failed.
	if r.opts.CheckUpstream && r.out.Live {
		for _, w := range checkUpstreamFreshness(r.cat, r.ws.RulePaths, r.model) {
			r.out.warnf("warning: %s\n", w)
		}
	}
}

// phases runs the observe-only arm. runErr is a run outcome recorded on the
// report (incomplete inventory); err aborts the run.
func (r *modelRun) phases(report *checkreport.ModelReport) (runErr error, err error) {
	r.out.progress("observe", r.model.Name, "started")
	model := r.model
	report.Mode = "observe-only"
	// A model with `desired` flags drift; without it, populate.
	switch {
	case len(model.Desired) > 0:
		// Inventory: populate, then set-diff desired against the records it
		// produced; --from-catalog replays already-ingested records instead.
		report.Mode = "observe-only (inventory)"
		policy, err := r.ws.InventoryPolicy()
		if err != nil {
			return nil, err
		}
		// Scope is mandatory on both arms: a live run compares against the
		// snapshot it just populated, and a replay compares against the
		// scope the operator named (ValidateOptions requires one). An
		// empty scope would query every observe record in the catalog.
		var scope string
		if r.opts.FromCatalog {
			scope = strings.TrimSpace(r.opts.CatalogScope)
		} else {
			pr, err := r.runPopulate()
			report.Populate = pr
			if err != nil {
				return nil, err
			}
			scope = pr.SnapshotID
			if scope == "" {
				return nil, fmt.Errorf("populate produced no snapshot to compare against")
			}
		}
		// This reads the records populate just wrote. Both borrow the run's
		// handle, so the read runs on the same connection as the write that
		// produced it.
		db, err := r.cat.required()
		if err != nil {
			return nil, err
		}
		res, err := inventory.Drift(r.ctx, db, scope, model.Desired, policy)
		if err != nil {
			return nil, err
		}
		// A replay is not an observation of the live system, so its verdict
		// cannot write a clean status.
		res.Verified = !r.opts.FromCatalog && !res.Uncertain
		report.Drift = &res
		// Failed inputs are why an inventory is incomplete, so they come first.
		if err := failedInputsError(report.Populate); err != nil {
			return err, nil
		}
		if res.Uncertain {
			return fmt.Errorf("incomplete inventory cannot establish absence; collect and select an explicitly complete snapshot"), nil
		}
	case model.Populate.Kind() == systemmodel.KindNone:
		// Checks-only: evaluate checks over what the catalog already holds.
		report.Mode = "checks-only"
	default:
		pr, err := r.runPopulate()
		report.Populate = pr
		if err != nil {
			return nil, err
		}
		return failedInputsError(pr), nil
	}
	return nil, nil
}

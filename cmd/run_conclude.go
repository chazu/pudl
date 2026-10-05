package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

// concludeRun finalizes a standalone run, then saves and renders its report.
// runErr is the phases' outcome error; the returned error is the run's final
// one. persisted reports whether the report was saved, so the caller's
// deferred save does not repeat it.
func concludeRun(in runPhaseInput, report *RunReport, finishState *runFinishState, runErr error, deps runDeps) (persisted bool, err error) {
	ctx := in.ctx
	if ctx == nil {
		ctx = runOperationContext(in.mu)
	}
	fin, err := finalizeRun(runFinalizeInput{
		cat: in.cat, model: in.model, effective: in.effective, modelDir: in.modelDir,
		runID: in.session.RunID, flags: in.flags, live: in.live, ctx: ctx,
	}, report, finishState, runErr)
	if err != nil {
		return false, err
	}

	persisted = persistRunReport(in.cat, report, in.live)
	out, err := report.render(jsonOutput)
	if err != nil {
		return persisted, err
	}
	if in.live && deps.set.emitOutput() {
		fmt.Fprint(outw(), "\n")
	}
	if deps.set.emitOutput() {
		fmt.Fprint(outw(), out)
	}
	if in.live {
		for _, notice := range fin.notices {
			fmt.Fprint(outw(), notice)
		}
	}
	return persisted, fin.runErr
}

// runFinalizeInput is what finalizeRun needs from a finished run.
type runFinalizeInput struct {
	ctx context.Context
	cat *runCatalog
	// model is the model as declared; effective is the scoped model the run
	// actually planned and executed (the same model when unscoped).
	model     *systemmodel.SystemModel
	effective *systemmodel.SystemModel
	modelDir  string
	runID     string
	flags     runFlags
	live      bool
}

// runFinalization is what finalizeRun concluded.
type runFinalization struct {
	// runErr is the phases' error, or a fail-severity check failure when the
	// phases succeeded.
	runErr  error
	verdict string
	// notices explain a verdict to the operator; callers print them after the
	// report.
	notices []string
}

// errFailSeverityChecks is the run outcome when every phase succeeded but a
// fail-severity check did not pass.
var errFailSeverityChecks = fmt.Errorf("one or more fail-severity checks did not pass")

// finalizeRun evaluates the model's checks, decides the run's verdict, and
// records it on the model instance row and the model's resources. It is the one
// conclusion shared by standalone runs and run-set converge members, so a set
// member is checked, recorded and promoted exactly as a standalone run is.
//
// The verdict and its explanations land in state for the run row. err reports
// a failure to evaluate the checks at all; nothing is recorded then.
func finalizeRun(in runFinalizeInput, report *RunReport, state *runFinishState, runErr error) (runFinalization, error) {
	cat, flags := in.cat, in.flags

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
	if len(in.effective.Checks) > 0 && !flags.dryRun {
		evalCtx := in.ctx
		if evalCtx == nil {
			evalCtx = context.Background()
		}
		results, err := runChecksContext(evalCtx, cat, in.effective, in.modelDir, checkContext{
			runID:       in.runID,
			fromCatalog: flags.fromCatalog,
			scope:       acute.NewTupleScope(in.model, in.effective),
		})
		if err != nil {
			return runFinalization{runErr: runErr}, err
		}
		report.Checks = results
		if anyFailSeverityFailed(results) {
			report.OK = false
			// First error wins: a converge failure is what the operator needs to
			// see, and a check failure must not displace it.
			if runErr == nil {
				runErr = errFailSeverityChecks
			}
		}
	}

	applyRunError(report, runErr)
	if runErr == nil {
		report.CompletionStatus = database.RunStatusSucceeded
	}
	fin := runFinalization{runErr: runErr}

	// Persist the run's terminal verdict on the model instance row so
	// `pudl model list` / `pudl status` surface last-run state, and record the
	// same conclusion on the run row. The run row is what tells an `unknown`
	// caused by a lost receipt apart from the `unknown` of a resource nobody has
	// ever observed — they are the same value on the model row by design.
	verdict := runVerdict(report, flags)
	fin.verdict = verdict
	state.verdict = verdict
	if report.Converge != nil {
		state.outcome = report.Converge.Outcome
		state.needsVerification = report.Converge.NeedsVerification
	}

	// The run row keeps the run's real verdict; the model instance row
	// describes the *whole* model, so a scoped run's verdict may not be
	// generalizable onto it. The note records the divergence so a reader can
	// tell this `unknown` from one caused by a lost receipt.
	// A verdict demoted by a check reads as ordinary resource drift on the model
	// row, so say which checks did it — otherwise an operator hunts for drift
	// that is not there.
	if names := failedFailSeverityNames(report.Checks); len(names) > 0 && verdict == "drifted" {
		state.addNote(fmt.Sprintf("verdict demoted to %q by fail-severity check(s): %s",
			verdict, strings.Join(names, ", ")))
		fin.notices = append(fin.notices, fmt.Sprintf("\nnote: fail-severity check(s) did not pass: %s\n"+
			"      the model's resources may match desired state; the failure is the check's assertion\n",
			strings.Join(names, ", ")))
	}

	restricted := len(flags.only) > 0
	rowVerdict := modelRowVerdict(verdict, restricted)
	if rowVerdict != verdict {
		note := fmt.Sprintf("verdict %q covers only the --only scope (%s); model status left %q",
			verdict, strings.Join(flags.only, ","), rowVerdict)
		state.addNote(note)
		fin.notices = append(fin.notices, fmt.Sprintf("\nnote: %s\n"+
			"      a scoped ∅ does not prove the whole model clean; re-run unscoped to establish it\n", note))
	}
	persistRunStatus(cat, in.model.Name, rowVerdict, in.live)

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
		promoteConvergingResources(cat, in.effective, restricted)
	}
	return fin, nil
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
			fmt.Fprintf(errw(), "warning: could not open catalog to record status %q: %v\n", status, err)
		}
		return
	}
	if err := db.UpdateStatus(modelTargetKey(name), status); err != nil && live {
		fmt.Fprintf(errw(), "warning: could not record status %q: %v\n", status, err)
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

package cmd

import (
	"context"
	"fmt"
	"strings"

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
		ctx = context.Background()
	}
	fin, err := finalizeRun(runFinalizeInput{
		cat: in.cat, model: in.model, modelDir: in.modelDir,
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
		if _, err := fmt.Fprint(outw(), out); err != nil {
			return persisted, err
		}
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
	ctx      context.Context
	cat      *runCatalog
	model    *systemmodel.SystemModel
	modelDir string
	runID    string
	flags    runFlags
	live     bool
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
var errCheckEvidence = fmt.Errorf("one or more checks could not establish a verdict; inspect check diagnostics")

// finalizeRun evaluates the model's checks, decides the run's verdict, and
// records it on the model instance row.
//
// The verdict and its explanations land in state for the run row. err reports
// a failure to evaluate the checks at all; nothing is recorded then.
func finalizeRun(in runFinalizeInput, report *RunReport, state *runFinishState, runErr error) (runFinalization, error) {
	cat, flags := in.cat, in.flags

	if len(in.model.Checks) > 0 {
		evalCtx := in.ctx
		if evalCtx == nil {
			evalCtx = context.Background()
		}
		currentSnapshot := flags.catalogScope
		if !flags.fromCatalog {
			if report.Populate != nil {
				currentSnapshot = report.Populate.SnapshotID
			}
			if report.Drift != nil && report.Drift.SnapshotID != "" {
				currentSnapshot = report.Drift.SnapshotID
			}
		}
		results, err := runChecksContext(evalCtx, cat, in.model, in.modelDir, checkContext{
			currentSnapshot: currentSnapshot,
			runID:           in.runID,
			fromCatalog:     flags.fromCatalog,
		})
		if err != nil {
			return runFinalization{runErr: runErr}, err
		}
		report.Checks = results
		if anyCheckUncertain(results) {
			report.OK = false
			if runErr == nil {
				runErr = errCheckEvidence
			}
		} else if anyFailSeverityFailed(results) {
			report.OK = false
			// First error wins: a phase failure is what the operator needs to
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
	// same conclusion on the run row.
	verdict := runVerdict(report)
	fin.verdict = verdict
	state.verdict = verdict

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
	persistRunStatus(cat, in.model.Name, verdict, in.live)
	return fin, nil
}

// runVerdict maps a finished run to a catalog status, or "" when none applies:
// a pure populate has no drift verdict.
//
// "clean" is the single in-sync verdict (drift == ∅). It is only ever written
// off an actual ∅ observation.
func runVerdict(r *RunReport) string {
	if r.Drift != nil && r.Drift.Uncertain {
		return "unknown"
	}
	if anyCheckUncertain(r.Checks) {
		return "unknown"
	}
	verdict := phaseVerdict(r)
	// A fail-severity check that did not pass says the model is not in the state
	// it declares, so a `clean` written over it would claim verification the run
	// contradicts. Only `clean` is demoted: `drifted` and `failed` are already at
	// least as severe, `unknown` means the run could not prove the state at all,
	// and "" writes nothing by design.
	//
	// `drifted` rather than `failed`: the run's machinery worked. What failed is
	// an assertion about the observed state, which is what `drifted` names.
	if verdict == "clean" && anyFailSeverityFailed(r.Checks) {
		return "drifted"
	}
	return verdict
}

func anyCheckUncertain(results []CheckResult) bool {
	for _, c := range results {
		if c.Outcome == "unknown" || c.Outcome == "error" {
			return true
		}
	}
	return false
}

// phaseVerdict is the verdict the run's phases alone support, before checks are
// allowed to demote it.
func phaseVerdict(r *RunReport) string {
	if r.Drift == nil {
		return ""
	}
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

// anyFailSeverityFailed reports whether any severity:"fail" check did not pass.
func anyFailSeverityFailed(results []CheckResult) bool {
	return len(failedFailSeverityNames(results)) > 0
}

// failedFailSeverityNames lists the fail-severity checks that did not pass.
func failedFailSeverityNames(results []CheckResult) []string {
	var names []string
	for _, c := range results {
		if !c.Passed && c.Severity == "fail" {
			names = append(names, c.Name)
		}
	}
	return names
}

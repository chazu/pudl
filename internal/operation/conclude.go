package operation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

// conclude finalizes a member run, then saves and renders its report. runErr
// is the phases' outcome error; the returned error is the run's final one.
// persisted reports whether the report was saved, so the caller's deferred
// save does not repeat it.
func (r *modelRun) conclude(report *checkreport.ModelReport, finishState *runFinishState, runErr error) (persisted bool, err error) {
	fin, err := r.finalize(report, finishState, runErr)
	if err != nil {
		return false, err
	}

	persisted = persistRunReport(r.cat, r.out, report)
	// Human output shows each member's report as it concludes; JSON output is
	// the check's one report, written when the check ends.
	if r.out.Live {
		shown := report
		if r.out.Owner != "" {
			shown = report.ForOwner(r.out.Owner)
		}
		out, err := shown.Render(false)
		if err != nil {
			return persisted, err
		}
		if _, err := fmt.Fprint(r.out.Out, "\n"+out); err != nil {
			return persisted, err
		}
		for _, notice := range fin.notices {
			fmt.Fprint(r.out.Out, notice)
		}
	}
	return persisted, fin.runErr
}

// runFinalization is what finalize concluded.
type runFinalization struct {
	// runErr is the phases' error, or a fail-severity check failure when the
	// phases succeeded.
	runErr  error
	verdict string
	// notices explain a verdict to the operator; callers print them after the
	// report.
	notices []string
}

// ErrFailSeverityChecks is a model's outcome when every phase succeeded but a
// fail-severity check did not pass.
var ErrFailSeverityChecks = fmt.Errorf("one or more fail-severity checks did not pass")
var errCheckEvidence = fmt.Errorf("one or more checks could not establish a verdict; inspect check diagnostics")

// finalize evaluates the model's checks, decides the run's verdict, and
// records it on the model instance row.
//
// The verdict and its explanations land in state for the run row. err reports
// a failure to evaluate the checks at all; nothing is recorded then.
func (r *modelRun) finalize(report *checkreport.ModelReport, state *runFinishState, runErr error) (runFinalization, error) {
	if len(r.model.Checks) > 0 {
		currentSnapshot := r.opts.CatalogScope
		if !r.opts.FromCatalog {
			if report.Populate != nil {
				currentSnapshot = report.Populate.SnapshotID
			}
			if report.Drift != nil && report.Drift.SnapshotID != "" {
				currentSnapshot = report.Drift.SnapshotID
			}
		}
		results, err := runChecks(r.ctx, r.cat, r.ws, r.out, r.model, r.modelDir, checkContext{
			currentSnapshot: currentSnapshot,
			runID:           r.session.RunID,
			fromCatalog:     r.opts.FromCatalog,
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
				runErr = ErrFailSeverityChecks
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
	persistRunStatus(r.cat, r.out, r.model.Name, verdict)
	return fin, nil
}

// runVerdict maps a finished run to a catalog status, or "" when none applies:
// a pure populate has no drift verdict.
//
// "clean" is the single in-sync verdict (drift == ∅). It is only ever written
// off an actual ∅ observation.
func runVerdict(r *checkreport.ModelReport) string {
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

func anyCheckUncertain(results []checkreport.CheckResult) bool {
	for _, c := range results {
		if c.Uncertain() {
			return true
		}
	}
	return false
}

// phaseVerdict is the verdict the run's phases alone support, before checks are
// allowed to demote it.
func phaseVerdict(r *checkreport.ModelReport) string {
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
// (target = systemmodel.InstanceTarget(name)). Best-effort: a status-write
// failure (or no catalog) never fails the run, but it is reported rather than
// swallowed — a silently dropped verdict leaves the previous run's status
// standing, which is how a stale `clean` used to survive.
func persistRunStatus(cat *runCatalog, out Output, name, status string) {
	if status == "" {
		return
	}
	db, err := cat.optional()
	if err != nil {
		out.warnf("warning: could not open catalog to record status %q: %v\n", status, err)
		return
	}
	if err := db.UpdateStatus(systemmodel.InstanceTarget(name), status); err != nil {
		out.warnf("warning: could not record status %q: %v\n", status, err)
	}
}

// anyFailSeverityFailed reports whether any severity:"fail" check did not pass.
func anyFailSeverityFailed(results []checkreport.CheckResult) bool {
	return len(failedFailSeverityNames(results)) > 0
}

// failedFailSeverityNames lists the fail-severity checks that did not pass.
func failedFailSeverityNames(results []checkreport.CheckResult) []string {
	var names []string
	for _, c := range results {
		if !c.Passed && c.Severity == "fail" {
			names = append(names, c.Name)
		}
	}
	return names
}

// applyRunError copies a phase error into the report before it is rendered or
// persisted. Reports are persisted before the check returns, so relying only
// on the deferred named-return finalizer would lose the error details.
func applyRunError(report *checkreport.ModelReport, err error) {
	if report == nil || err == nil {
		return
	}
	report.OK = false
	report.Error = err.Error()
	report.CompletionStatus = failureStatus(err)
}

// persistRunReport saves a model's report with its summary. Best-effort: a
// failure is a warning, and false says the report was not saved.
func persistRunReport(cat *runCatalog, out Output, report *checkreport.ModelReport) bool {
	if report == nil || report.RunID == "" {
		return false
	}
	if report.ReportVersion == 0 {
		report.ReportVersion = checkreport.ModelReportVersion
	}
	db, err := cat.optional()
	if err != nil {
		out.warnf("warning: could not open catalog to persist run report: %v\n", err)
		return false
	}
	report.Summary = checkreport.Summarize(report, db)
	b, err := json.Marshal(report)
	if err != nil {
		out.warnf("warning: could not encode run report: %v\n", err)
		return false
	}
	if err := db.SaveRunReport(report.RunID, report.Model, b); err != nil {
		out.warnf("warning: could not persist run report: %v\n", err)
		return false
	}
	return true
}

// reportHasFindings reports whether a concluded run found something an
// operator must act on: drift or a failing fail-severity check.
func reportHasFindings(r *checkreport.ModelReport) bool {
	if r == nil {
		return false
	}
	if r.Drift != nil && !r.Drift.Clean {
		return true
	}
	return anyFailSeverityFailed(r.Checks)
}

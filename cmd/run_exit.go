package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	pudlerrors "github.com/chazu/pudl/internal/errors"
)

// Exit codes under --detailed-exitcode, after `terraform plan -detailed-exitcode`
// and `git diff --exit-code`: a script can tell "in sync" from "found
// something" from "could not tell" without parsing the report.
const (
	exitClean    = 0
	exitError    = 1
	exitFindings = 2
)

const detailedExitCodeUsage = "exit 0 when clean, 2 when drift, pending changes or a failing fail-severity check was found, 1 on error"

// exitCodeError carries a specific process exit code out of a command. A nil
// err exits with the code and prints nothing.
type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit status %d", e.code)
	}
	return e.err.Error()
}

func (e *exitCodeError) Unwrap() error { return e.err }

// exitCodeFor maps a command's error to the process exit code: an explicit
// exitCodeError wins, then a PUDLError's own code, then 1.
func exitCodeFor(err error) int {
	if err == nil {
		return exitClean
	}
	var coded *exitCodeError
	if errors.As(err, &coded) {
		return coded.code
	}
	var pudlErr *pudlerrors.PUDLError
	if errors.As(err, &pudlErr) {
		return pudlErr.GetExitCode()
	}
	return exitError
}

// reportHasFindings reports whether a concluded run found something an
// operator must act on: drift, changes a dry run would apply, or a failing
// fail-severity check.
func reportHasFindings(r *RunReport) bool {
	if r == nil {
		return false
	}
	if r.Drift != nil && !r.Drift.Clean {
		return true
	}
	if r.Converge != nil && r.Converge.Outcome == string(outcomeDryRun) {
		return true
	}
	return anyFailSeverityFailed(r.Checks)
}

// detailedRunExit maps a finished `pudl run` to the --detailed-exitcode
// contract. A run that fails only because a fail-severity check did not pass
// found something (2); any other error is 1, whatever its own code would be,
// so 2 always means findings.
func detailedRunExit(report *RunReport, err error) error {
	switch {
	case err == nil && !reportHasFindings(report):
		return nil
	case err == nil || errors.Is(err, errFailSeverityChecks):
		return &exitCodeError{code: exitFindings, err: err}
	default:
		return &exitCodeError{code: exitError, err: err}
	}
}

// detailedRunSetExit maps a finished `pudl run set` to the same contract. An
// observe-only set exits 2 when any member found drift or a failing check; a
// converging set succeeds only once every member converged clean. A failed set
// exits 2 only when every failed member failed on its checks alone — blocked
// and cancelled members are consequences of those failures, not new errors.
func detailedRunSetExit(result *runSetResult, err error) error {
	if err == nil {
		if result != nil && result.findings && result.report.Mode == "observe-only" {
			return &exitCodeError{code: exitFindings}
		}
		return nil
	}
	if result != nil && onlyCheckFailures(result.report) {
		return &exitCodeError{code: exitFindings, err: err}
	}
	return &exitCodeError{code: exitError, err: err}
}

func onlyCheckFailures(report *acute.RunSetReport) bool {
	if report == nil {
		return false
	}
	failed := 0
	for _, member := range report.Members {
		if member.Result != database.RunStatusFailed {
			continue
		}
		if member.Error != errFailSeverityChecks.Error() {
			return false
		}
		failed++
	}
	return failed > 0
}

// silenceBareExit stops cobra printing "Error:" for an exit that carries no
// error message, so a successful run that found drift prints its report and
// nothing else.
func silenceBareExit(cmd *cobra.Command, err error) error {
	var coded *exitCodeError
	if errors.As(err, &coded) && coded.err == nil {
		cmd.SilenceErrors = true
	}
	return err
}

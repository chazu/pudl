package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	pudlerrors "github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/operation"
)

// Exit codes under --detailed-exitcode, after `terraform plan -detailed-exitcode`
// and `git diff --exit-code`: a script can tell "in sync" from "found
// something" from "could not tell" without parsing the report.
const (
	exitClean    = 0
	exitError    = 1
	exitFindings = 2
)

// --fail-on values: which fail-severity check failures fail a check.
const (
	failOnAny = "any"
	failOnNew = "new"
)

const detailedExitCodeUsage = "exit 0 when clean, 2 when drift or a failing fail-severity check was found, 1 on error"

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

// detailedCheckExit maps a finished `pudl check` to the --detailed-exitcode
// contract. A check exits 2 when any model found drift or a failing check. A
// failed check exits 2 only when every failed model failed on its
// fail-severity checks alone — blocked and cancelled models are consequences
// of those failures, not new errors. Any other error is 1, whatever its own
// code would be, so 2 always means findings.
//
// With onlyNew (--fail-on new), fail-severity check failures whose matches
// earlier reports already recorded are not findings; drift still is.
func detailedCheckExit(result *operation.Result, err error, onlyNew bool) error {
	if err == nil {
		if result != nil && result.Findings {
			return &exitCodeError{code: exitFindings}
		}
		return nil
	}
	if onlyNew && result.OnlyKnownCheckFailures() {
		if result.Drifted {
			return &exitCodeError{code: exitFindings}
		}
		return nil
	}
	if result.OnlyCheckFailures() {
		return &exitCodeError{code: exitFindings, err: err}
	}
	return &exitCodeError{code: exitError, err: err}
}

// failOnExit is a check's error without --detailed-exitcode: with onlyNew,
// a check that failed only on known fail-severity check failures succeeds.
func failOnExit(result *operation.Result, err error, onlyNew bool) error {
	if onlyNew && err != nil && result.OnlyKnownCheckFailures() {
		return nil
	}
	return err
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

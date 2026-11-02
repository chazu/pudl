package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	pudlerrors "github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/operation"
)

func TestExitCodeForPrefersExplicitCodeThenPUDLError(t *testing.T) {
	assert.Equal(t, 0, exitCodeFor(nil))
	assert.Equal(t, 1, exitCodeFor(fmt.Errorf("plain")))
	assert.Equal(t, 2, exitCodeFor(&exitCodeError{code: 2}))
	assert.Equal(t, 2, exitCodeFor(fmt.Errorf("wrapped: %w", &exitCodeError{code: 2})))

	invalid := pudlerrors.NewInputError("bad flag")
	assert.Equal(t, invalid.GetExitCode(), exitCodeFor(invalid), "a RunE command's PUDLError keeps its own code")
	assert.Equal(t, invalid.GetExitCode(), exitCodeFor(fmt.Errorf("context: %w", invalid)))
	assert.Equal(t, 1, exitCodeFor(&exitCodeError{code: 1, err: invalid}),
		"an explicit code wins, so --detailed-exitcode's 2 is never an input error")
}

// singleModelCheck is the result of a check of one model that concluded
// (concluded false: failed before it had a report) with findings and err, as
// operation.Check reports it.
func singleModelCheck(concluded, findings bool, err error) (*operation.Result, error) {
	if !concluded {
		return nil, err
	}
	member := acute.RunSetMemberReport{Model: "m", Result: database.RunStatusSucceeded}
	if err != nil {
		member.Result = database.RunStatusFailed
		member.Error = err.Error()
		err = fmt.Errorf("check c failed: m failed: %w", err)
	}
	result := &operation.Result{Report: &acute.RunSetReport{Members: []acute.RunSetMemberReport{member}}, Findings: findings}
	return result, err
}

// detailedAnyExit is --detailed-exitcode with the default --fail-on any.
func detailedAnyExit(result *operation.Result, err error) error {
	return detailedCheckExit(result, err, false)
}

// TestFailOnNewExitsOnlyForNewFailures covers --fail-on new with and without
// --detailed-exitcode: known fail-severity failures pass, a new one, drift,
// a blocked model or any other error still count.
func TestFailOnNewExitsOnlyForNewFailures(t *testing.T) {
	checkFailed := acute.RunSetMemberReport{Model: "a", Result: database.RunStatusFailed, Error: operation.ErrFailSeverityChecks.Error()}
	blocked := acute.RunSetMemberReport{Model: "b", Result: database.RunStatusBlocked, Error: "blocked by unsuccessful prerequisite \"a\""}
	broken := acute.RunSetMemberReport{Model: "c", Result: database.RunStatusFailed, Error: "populate: exit 1"}
	clean := acute.RunSetMemberReport{Model: "d", Result: database.RunStatusSucceeded}
	checkErr := fmt.Errorf("check s failed: %w", operation.ErrFailSeverityChecks)
	result := func(newFailures, drifted bool, members ...acute.RunSetMemberReport) *operation.Result {
		return &operation.Result{Report: &acute.RunSetReport{Members: members}, Findings: true, NewFailures: newFailures, Drifted: drifted}
	}
	cases := []struct {
		name            string
		result          *operation.Result
		err             error
		detailed, plain int
		detailedAny     int
	}{
		{"known failures only", result(false, false, checkFailed, clean), checkErr, 0, 0, 2},
		{"a new failure", result(true, false, checkFailed), checkErr, 2, 1, 2},
		{"known failures with drift", result(false, true, checkFailed), checkErr, 2, 0, 2},
		{"known failures and a blocked model", result(false, false, checkFailed, blocked), checkErr, 2, 1, 2},
		{"known failures and an error", result(false, false, checkFailed, broken), checkErr, 1, 1, 1},
		{"drift alone", result(false, true, clean), nil, 2, 0, 2},
		{"error before a report", nil, fmt.Errorf("resolve model"), 1, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.detailed, exitCodeFor(detailedCheckExit(tc.result, tc.err, true)), "--fail-on new --detailed-exitcode")
			assert.Equal(t, tc.plain, exitCodeFor(failOnExit(tc.result, tc.err, true)), "--fail-on new")
			assert.Equal(t, tc.detailedAny, exitCodeFor(detailedCheckExit(tc.result, tc.err, false)), "--fail-on any --detailed-exitcode")
		})
	}
}

func TestDetailedCheckExitForOneModel(t *testing.T) {
	cases := []struct {
		name      string
		concluded bool
		findings  bool
		err       error
		code      int
	}{
		{"clean drift", true, false, nil, 0},
		{"populate only", true, false, nil, 0},
		{"drift", true, true, nil, 2},
		{"failing check", true, true, operation.ErrFailSeverityChecks, 2},
		{"populate failure", true, false, fmt.Errorf("command exited 1"), 1},
		{"preflight error", false, false, pudlerrors.NewInputError("bad"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, exitCodeFor(detailedAnyExit(singleModelCheck(tc.concluded, tc.findings, tc.err))))
		})
	}
}

func TestDetailedCheckExitKeepsTheErrorMessage(t *testing.T) {
	result, checkErr := singleModelCheck(true, true, operation.ErrFailSeverityChecks)
	err := detailedAnyExit(result, checkErr)
	assert.True(t, errors.Is(err, operation.ErrFailSeverityChecks))
	assert.Equal(t, checkErr.Error(), err.Error())
}

func TestDetailedCheckExitForSeveralModels(t *testing.T) {
	observe := func(findings bool, members ...acute.RunSetMemberReport) *operation.Result {
		return &operation.Result{Report: &acute.RunSetReport{Mode: "observe-only", Members: members}, Findings: findings}
	}
	checkFailed := acute.RunSetMemberReport{Model: "a", Result: database.RunStatusFailed, Error: operation.ErrFailSeverityChecks.Error()}
	blocked := acute.RunSetMemberReport{Model: "b", Result: database.RunStatusBlocked, Error: "blocked by unsuccessful prerequisite \"a\""}
	broken := acute.RunSetMemberReport{Model: "c", Result: database.RunStatusFailed, Error: "populate: exit 1"}
	setFailed := fmt.Errorf("check s failed")

	assert.Equal(t, 0, exitCodeFor(detailedAnyExit(observe(false), nil)))
	assert.Equal(t, 2, exitCodeFor(detailedAnyExit(observe(true), nil)))

	assert.Equal(t, 2, exitCodeFor(detailedAnyExit(observe(true, checkFailed, blocked), setFailed)))
	assert.Equal(t, 1, exitCodeFor(detailedAnyExit(observe(true, checkFailed, broken), setFailed)))
	assert.Equal(t, 1, exitCodeFor(detailedAnyExit(nil, fmt.Errorf("resolve model"))))
}

func TestSilenceBareExitOnlySilencesMessagelessExits(t *testing.T) {
	quiet := &cobra.Command{}
	assert.Error(t, silenceBareExit(quiet, &exitCodeError{code: 2}))
	assert.True(t, quiet.SilenceErrors)

	loud := &cobra.Command{}
	_ = silenceBareExit(loud, &exitCodeError{code: 1, err: fmt.Errorf("boom")})
	assert.False(t, loud.SilenceErrors)
}

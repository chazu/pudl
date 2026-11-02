package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	pudlerrors "github.com/chazu/pudl/internal/errors"
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

// singleModelCheck is the result of a check of one model that concluded with
// report and err, as executeCheck reports it.
func singleModelCheck(report *RunReport, err error) (*checkResult, error) {
	if report == nil {
		return nil, err
	}
	member := acute.RunSetMemberReport{Model: "m", Result: database.RunStatusSucceeded}
	if err != nil {
		member.Result = database.RunStatusFailed
		member.Error = err.Error()
		err = fmt.Errorf("check c failed: m failed: %w", err)
	}
	result := &checkResult{report: &acute.RunSetReport{Members: []acute.RunSetMemberReport{member}}, findings: reportHasFindings(report)}
	return result, err
}

func TestDetailedCheckExitForOneModel(t *testing.T) {
	clean := &RunReport{Drift: &ModelDriftResult{Clean: true, Verified: true}}
	drifted := &RunReport{Drift: &ModelDriftResult{Clean: false, Verified: true}}
	replayDrift := &RunReport{Drift: &ModelDriftResult{Clean: false, Verified: false}}
	failedCheck := &RunReport{Checks: []CheckResult{{Name: "c", Severity: "fail", Passed: false}}}
	warnCheck := &RunReport{Checks: []CheckResult{{Name: "c", Severity: "warn", Passed: false}}}

	cases := []struct {
		name   string
		report *RunReport
		err    error
		code   int
	}{
		{"clean drift", clean, nil, 0},
		{"populate only", &RunReport{}, nil, 0},
		{"warn-severity check", warnCheck, nil, 0},
		{"drift", drifted, nil, 2},
		{"replayed drift", replayDrift, nil, 2},
		{"failing check", failedCheck, errFailSeverityChecks, 2},
		{"populate failure", &RunReport{}, fmt.Errorf("command exited 1"), 1},
		{"preflight error", nil, pudlerrors.NewInputError("bad"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, exitCodeFor(detailedCheckExit(singleModelCheck(tc.report, tc.err))))
		})
	}
}

func TestDetailedCheckExitKeepsTheErrorMessage(t *testing.T) {
	result, checkErr := singleModelCheck(&RunReport{}, errFailSeverityChecks)
	err := detailedCheckExit(result, checkErr)
	assert.True(t, errors.Is(err, errFailSeverityChecks))
	assert.Equal(t, checkErr.Error(), err.Error())
}

func TestDetailedCheckExitForSeveralModels(t *testing.T) {
	observe := func(findings bool, members ...acute.RunSetMemberReport) *checkResult {
		return &checkResult{report: &acute.RunSetReport{Mode: "observe-only", Members: members}, findings: findings}
	}
	checkFailed := acute.RunSetMemberReport{Model: "a", Result: database.RunStatusFailed, Error: errFailSeverityChecks.Error()}
	blocked := acute.RunSetMemberReport{Model: "b", Result: database.RunStatusBlocked, Error: "blocked by unsuccessful prerequisite \"a\""}
	broken := acute.RunSetMemberReport{Model: "c", Result: database.RunStatusFailed, Error: "populate: exit 1"}
	setFailed := fmt.Errorf("check s failed")

	assert.Equal(t, 0, exitCodeFor(detailedCheckExit(observe(false), nil)))
	assert.Equal(t, 2, exitCodeFor(detailedCheckExit(observe(true), nil)))

	assert.Equal(t, 2, exitCodeFor(detailedCheckExit(observe(true, checkFailed, blocked), setFailed)))
	assert.Equal(t, 1, exitCodeFor(detailedCheckExit(observe(true, checkFailed, broken), setFailed)))
	assert.Equal(t, 1, exitCodeFor(detailedCheckExit(nil, fmt.Errorf("resolve model"))))
}

func TestDetailedCheckExitThroughRealChecks(t *testing.T) {
	setupCheckFixture(t)

	result, err := executeCheck(context.Background(), []string{"producer"}, checkOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCodeFor(detailedCheckExit(result, err)), "a populate-only member finds nothing")

	result, err = executeCheck(context.Background(), []string{"drifter"}, checkOptions{})
	assert.NoError(t, err, "observe-only drift is a result, not an error")
	assert.True(t, result.findings)
	assert.Equal(t, 2, exitCodeFor(detailedCheckExit(result, err)))
}

func TestSilenceBareExitOnlySilencesMessagelessExits(t *testing.T) {
	quiet := &cobra.Command{}
	assert.Error(t, silenceBareExit(quiet, &exitCodeError{code: 2}))
	assert.True(t, quiet.SilenceErrors)

	loud := &cobra.Command{}
	_ = silenceBareExit(loud, &exitCodeError{code: 1, err: fmt.Errorf("boom")})
	assert.False(t, loud.SilenceErrors)
}

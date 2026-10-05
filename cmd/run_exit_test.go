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

func TestDetailedRunExit(t *testing.T) {
	clean := &RunReport{Drift: &ModelDriftResult{Clean: true, Verified: true}}
	drifted := &RunReport{Drift: &ModelDriftResult{Clean: false, Verified: true}}
	replayDrift := &RunReport{Drift: &ModelDriftResult{Clean: false, Verified: false}}
	dryRunPending := &RunReport{Converge: &ConvergeReport{Outcome: string(outcomeDryRun)}}
	converged := &RunReport{Converge: &ConvergeReport{Outcome: string(outcomeClean)}}
	failedCheck := &RunReport{Checks: []CheckResult{{Name: "c", Severity: "fail", Passed: false}}}
	warnCheck := &RunReport{Checks: []CheckResult{{Name: "c", Severity: "warn", Passed: false}}}

	cases := []struct {
		name   string
		report *RunReport
		err    error
		code   int
	}{
		{"clean drift", clean, nil, 0},
		{"converged clean", converged, nil, 0},
		{"populate only", &RunReport{}, nil, 0},
		{"pending approval", &RunReport{PendingApproval: true}, nil, 0},
		{"warn-severity check", warnCheck, nil, 0},
		{"drift", drifted, nil, 2},
		{"replayed drift", replayDrift, nil, 2},
		{"dry run with pending changes", dryRunPending, nil, 2},
		{"failing check", failedCheck, errFailSeverityChecks, 2},
		{"converge failure", &RunReport{}, fmt.Errorf("convergence ended failed (cap_exhausted)"), 1},
		{"preflight error", nil, pudlerrors.NewInputError("bad"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, exitCodeFor(detailedRunExit(tc.report, tc.err)))
		})
	}
}

func TestDetailedRunExitKeepsTheErrorMessage(t *testing.T) {
	err := detailedRunExit(&RunReport{}, errFailSeverityChecks)
	assert.True(t, errors.Is(err, errFailSeverityChecks))
	assert.Equal(t, errFailSeverityChecks.Error(), err.Error())
}

func TestDetailedRunSetExit(t *testing.T) {
	observe := func(findings bool, members ...acute.RunSetMemberReport) *runSetResult {
		return &runSetResult{report: &acute.RunSetReport{Mode: "observe-only", Members: members}, findings: findings}
	}
	converge := &runSetResult{report: &acute.RunSetReport{Mode: "converge"}, findings: true}
	checkFailed := acute.RunSetMemberReport{Model: "a", Result: database.RunStatusFailed, Error: errFailSeverityChecks.Error()}
	blocked := acute.RunSetMemberReport{Model: "b", Result: database.RunStatusBlocked, Error: "blocked by unsuccessful prerequisite \"a\""}
	broken := acute.RunSetMemberReport{Model: "c", Result: database.RunStatusFailed, Error: "mu observe: exit 1"}
	setFailed := fmt.Errorf("run set s failed")

	assert.Equal(t, 0, exitCodeFor(detailedRunSetExit(observe(false), nil)))
	assert.Equal(t, 2, exitCodeFor(detailedRunSetExit(observe(true), nil)))
	assert.Equal(t, 0, exitCodeFor(detailedRunSetExit(converge, nil)),
		"a converging set that succeeded closed the drift its preflight found")
	assert.Equal(t, 2, exitCodeFor(detailedRunSetExit(observe(true, checkFailed, blocked), setFailed)))
	assert.Equal(t, 1, exitCodeFor(detailedRunSetExit(observe(true, checkFailed, broken), setFailed)))
	assert.Equal(t, 1, exitCodeFor(detailedRunSetExit(nil, fmt.Errorf("resolve model"))))
}

func TestRunSetDetailedExitThroughRealSets(t *testing.T) {
	h, _ := setupMutatingRunSetFixture(t, false)

	h.opts.converge = false
	result, err := executeRunSet([]string{"producer"}, h.opts, h.deps)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCodeFor(detailedRunSetExit(result, err)), "a populate-only member finds nothing")

	result, err = executeRunSet([]string{"mutator-a"}, h.opts, h.deps)
	assert.NoError(t, err, "observe-only drift is a result, not an error")
	assert.True(t, result.findings)
	assert.Equal(t, 2, exitCodeFor(detailedRunSetExit(result, err)))

	h.opts.converge = true
	result, err = executeRunSet([]string{"mutator-a"}, h.opts, h.deps)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCodeFor(detailedRunSetExit(result, err)), "converging closed the drift")
}

func TestSilenceBareExitOnlySilencesMessagelessExits(t *testing.T) {
	quiet := &cobra.Command{}
	assert.Error(t, silenceBareExit(quiet, &exitCodeError{code: 2}))
	assert.True(t, quiet.SilenceErrors)

	loud := &cobra.Command{}
	_ = silenceBareExit(loud, &exitCodeError{code: 1, err: fmt.Errorf("boom")})
	assert.False(t, loud.SilenceErrors)
}

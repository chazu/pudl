package cmd

import (
	"encoding/json"
	"testing"

	"github.com/chazu/pudl/internal/acute"
	"github.com/stretchr/testify/require"
)

func TestReportSummaryDistinguishesReplayAndUncertainMutation(t *testing.T) {
	r := &RunReport{RunID: "r", CompletionStatus: "succeeded", OK: true, Drift: &ModelDriftResult{Clean: false, Verified: false, Drifted: []ResourceDrift{{Resource: "x"}}}}
	s := summarizeRun(r, nil)
	require.Equal(t, "succeeded", s.Execution)
	require.Equal(t, "drifted", s.Conformity)
	require.Equal(t, "recorded", s.Verification)
	r.Converge = &ConvergeReport{NeedsVerification: true}
	s = summarizeRun(r, nil)
	require.Equal(t, "needs-verification", s.Verification)
	r.Converge = nil
	r.Drift = &ModelDriftResult{Clean: true, Verified: true}
	r.Checks = []CheckResult{{Outcome: "unknown", Severity: "warn"}}
	s = summarizeRun(r, nil)
	require.Equal(t, "not-verified", s.Verification)
	require.Equal(t, 1, s.UnknownChecks)
}

func TestSetSummaryIncludesMemberEvidenceWithoutFollowupRequests(t *testing.T) {
	db, member, mu := setConvergeFixture(t)
	report, err := executeSetMember(t, db, mu, member)
	require.NoError(t, err)
	require.NotNil(t, report.Summary)
	require.Len(t, report.Members, 1)
	require.NotNil(t, report.Members[0].Summary)
	require.Equal(t, "clean", report.Summary.Conformity)
	require.Equal(t, "pass", report.Summary.Checks)
	require.Equal(t, "live", report.Summary.Verification)
	stored, err := db.GetRunSetReport(report.RunSetID)
	require.NoError(t, err)
	var persisted acute.RunSetReport
	require.NoError(t, json.Unmarshal(stored.Report, &persisted))
	require.Equal(t, report.Summary, persisted.Summary)
}

func TestProgressDoesNotContaminateResultJSON(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "run", "missing", "--json", "--progress-json")
	require.Error(t, r.Err)
	require.True(t, json.Valid([]byte(r.Stdout)), r.Stdout)
	require.Contains(t, r.Stderr, `"type":"progress"`)
}

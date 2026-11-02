package cmd

import (
	"encoding/json"
	"testing"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
)

func TestReportSummaryDistinguishesReplayAndUncertainChecks(t *testing.T) {
	r := &RunReport{RunID: "r", CompletionStatus: "succeeded", OK: true, Drift: &ModelDriftResult{Clean: false, Verified: false, Drifted: []ResourceDrift{{Resource: "x"}}}}
	s := summarizeRun(r, nil)
	require.Equal(t, "succeeded", s.Execution)
	require.Equal(t, "drifted", s.Conformity)
	require.Equal(t, "recorded", s.Verification)
	r.Drift = &ModelDriftResult{Clean: true, Verified: true}
	r.Checks = []CheckResult{{Outcome: "unknown", Severity: "warn"}}
	s = summarizeRun(r, nil)
	require.Equal(t, "not-verified", s.Verification)
	require.Equal(t, 1, s.UnknownChecks)
}

func TestSetSummaryIncludesMemberEvidenceWithoutFollowupRequests(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	member := &RunReport{
		ReportVersion: 2, RunID: "run_member", Model: "m", Mode: "observe-only",
		CompletionStatus: database.RunStatusSucceeded, OK: true,
		Drift:  &ModelDriftResult{Clean: true, Verified: true},
		Checks: []CheckResult{{Name: "c", Severity: "fail", Passed: true, Outcome: "pass"}},
	}
	payload, err := json.Marshal(member)
	require.NoError(t, err)
	require.NoError(t, db.SaveRunReport(member.RunID, member.Model, payload))
	report := &acute.RunSetReport{
		RunSetID: "runset_x", Mode: "observe-only", Status: database.RunStatusSucceeded,
		Ordered: []string{"m"}, Members: []acute.RunSetMemberReport{{Model: "m", RunID: "run_member", Result: database.RunStatusSucceeded}},
	}
	require.NoError(t, saveCheckReport(db, report))
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
	require.Equal(t, checkReportVersion, persisted.ReportVersion)
	require.JSONEq(t, string(payload), string(persisted.Members[0].Report), "the check report embeds the member's full report")
}

func TestProgressDoesNotContaminateResultJSON(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "check", "missing", "--json", "--progress-json")
	require.Error(t, r.Err)
	require.True(t, json.Valid([]byte(r.Stdout)), r.Stdout)
	require.Contains(t, r.Stderr, `"type":"progress"`)
}

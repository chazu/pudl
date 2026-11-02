package operation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
)

func TestPersistRunReportUsesTheRunCatalog(t *testing.T) {
	cat := newRunCatalog(t.TempDir())
	defer cat.Close()

	report := &checkreport.ModelReport{
		RunID: "run_parity", Model: "pods", Mode: "observe-only", OK: false,
		Error: "observer failed after startup",
	}
	require.True(t, persistRunReport(cat, quietOutput(), report))

	db, err := cat.required()
	require.NoError(t, err)
	stored, err := db.GetRunReport("run_parity")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "pods", stored.Model)
	assert.Contains(t, string(stored.Report), "observer failed after startup")
}

func TestSetSummaryIncludesMemberEvidenceWithoutFollowupRequests(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	member := &checkreport.ModelReport{
		ReportVersion: 2, RunID: "run_member", Model: "m", Mode: "observe-only",
		CompletionStatus: database.RunStatusSucceeded, OK: true,
		Drift:  &acute.ModelDriftResult{Clean: true, Verified: true},
		Checks: []checkreport.CheckResult{{Name: "c", Severity: "fail", Passed: true, Outcome: "pass"}},
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
	require.Equal(t, ReportVersion, persisted.ReportVersion)
	require.JSONEq(t, string(payload), string(persisted.Members[0].Report), "the check report embeds the member's full report")
}

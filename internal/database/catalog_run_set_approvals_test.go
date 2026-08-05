package database

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSetApprovalIsImmutableAndDigestGuarded(t *testing.T) {
	db := runsTestDB(t)
	request := []byte(`{"models":["network","app"],"max_iters":5}`)
	plan := []byte(`{"plan_version":1,"run_set_id":"set_a"}`)
	require.NoError(t, db.SaveRunSetApproval("set_a", "digest_a", request, plan))
	assert.Error(t, db.SaveRunSetApproval("set_a", "digest_b", request, plan), "one identity cannot be recycled for another plan")

	record, err := db.GetRunSetApproval("set_a")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "digest_a", record.PlanDigest)
	assert.Equal(t, "pending", record.Status)
	assert.JSONEq(t, string(request), string(record.Request))
	assert.JSONEq(t, string(plan), string(record.Plan))
	assert.Nil(t, record.ResolvedAt)

	assert.Error(t, db.ResolveRunSetApproval("set_a", "digest_b", "approved"), "a changed plan cannot consume approval")
	require.NoError(t, db.ResolveRunSetApproval("set_a", "digest_a", "stale"))
	record, err = db.GetRunSetApproval("set_a")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "stale", record.Status)
	assert.NotNil(t, record.ResolvedAt)
	assert.Error(t, db.ResolveRunSetApproval("set_a", "digest_a", "approved"), "terminal approval cannot race or resolve twice")
}

func TestSavePendingRunSetApprovalCommitsGateMembersAndPinsAtomically(t *testing.T) {
	db := runsTestDB(t)
	require.NoError(t, db.StartRun("run_a", "model-a", "observe-only"))
	require.NoError(t, db.FinishRun("run_a", RunConclusion{CompletionStatus: RunStatusSucceeded}))
	require.NoError(t, db.SaveRunReport("run_a", "model-a", []byte(`{"status":"succeeded"}`)))
	require.NoError(t, db.SaveRunSetReport("set_a", []byte(`{"status":"running"}`)))
	require.NoError(t, db.RecordObserveSnapshot(ObserveSnapshot{
		SnapshotID: "snap_a", RunID: "run_a", Model: "model-a", Workspace: "repo",
		Origin: "pudl-run", Source: SnapshotSourceMuObserve, CreatedAt: time.Now(),
	}))

	err := db.SavePendingRunSetApproval(PendingRunSetApproval{
		RunSetID: "set_a", PlanDigest: "digest_a",
		Request: []byte(`{"models":["model-a"]}`), Plan: []byte(`{"plan_version":1}`),
		Report: []byte(`{"status":"pending-approval"}`), SnapshotIDs: []string{"snap_a"},
		Members: []PendingRunSetMember{{
			RunID: "run_a", Model: "model-a", Report: []byte(`{"status":"running","pending_approval":true}`),
		}},
	})
	require.NoError(t, err)

	run, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, RunStatusRunning, run.CompletionStatus)
	assert.Equal(t, "converge", run.Mode)
	member, err := db.GetRunReport("run_a")
	require.NoError(t, err)
	require.NotNil(t, member)
	assert.JSONEq(t, `{"status":"running","pending_approval":true}`, string(member.Report))
	set, err := db.GetRunSetReport("set_a")
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.JSONEq(t, `{"status":"pending-approval"}`, string(set.Report))
	approval, err := db.GetRunSetApproval("set_a")
	require.NoError(t, err)
	require.NotNil(t, approval)
	assert.Equal(t, "pending", approval.Status)
	snapshot, err := db.GetObserveSnapshot("snap_a")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.True(t, snapshot.Retained)
}

func TestSavePendingRunSetApprovalRollsBackEverySurfaceOnFailure(t *testing.T) {
	db := runsTestDB(t)
	require.NoError(t, db.StartRun("run_a", "model-a", "observe-only"))
	require.NoError(t, db.FinishRun("run_a", RunConclusion{CompletionStatus: RunStatusSucceeded}))
	require.NoError(t, db.SaveRunReport("run_a", "model-a", []byte(`{"status":"succeeded"}`)))
	require.NoError(t, db.SaveRunSetReport("set_a", []byte(`{"status":"running"}`)))
	require.NoError(t, db.RecordObserveSnapshot(ObserveSnapshot{
		SnapshotID: "snap_a", RunID: "run_a", Model: "model-a", Workspace: "repo",
		Origin: "pudl-run", Source: SnapshotSourceMuObserve, CreatedAt: time.Now(),
	}))

	err := db.SavePendingRunSetApproval(PendingRunSetApproval{
		RunSetID: "set_a", PlanDigest: "digest_a",
		Request: []byte(`{"models":["model-a"]}`), Plan: []byte(`{"plan_version":1}`),
		Report: []byte(`{"status":"pending-approval"}`), SnapshotIDs: []string{"snap_a", "missing"},
		Members: []PendingRunSetMember{{
			RunID: "run_a", Model: "model-a", Report: []byte(`{"status":"running"}`),
		}},
	})
	require.ErrorContains(t, err, "missing")

	run, err := db.GetRun("run_a")
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, RunStatusSucceeded, run.CompletionStatus)
	member, err := db.GetRunReport("run_a")
	require.NoError(t, err)
	require.NotNil(t, member)
	assert.JSONEq(t, `{"status":"succeeded"}`, string(member.Report))
	set, err := db.GetRunSetReport("set_a")
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.JSONEq(t, `{"status":"running"}`, string(set.Report))
	approval, err := db.GetRunSetApproval("set_a")
	require.NoError(t, err)
	assert.Nil(t, approval)
	snapshot, err := db.GetObserveSnapshot("snap_a")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.False(t, snapshot.Retained)
}

func TestRunSetApprovalCanApproveOnlyMatchingPendingPlan(t *testing.T) {
	db := runsTestDB(t)
	require.NoError(t, db.SaveRunSetApproval("set_b", "digest_b", []byte(`{"models":["app"]}`), []byte(`{"plan_version":1}`)))
	require.NoError(t, db.ResolveRunSetApproval("set_b", "digest_b", "approved"))
	record, err := db.GetRunSetApproval("set_b")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "approved", record.Status)
}

func TestApproveRunSetPlanCASAndReportAreAtomic(t *testing.T) {
	db := runsTestDB(t)
	require.NoError(t, db.SaveRunSetApproval("set_c", "digest_c", []byte(`{"models":["app"]}`), []byte(`{"plan_version":1}`)))
	require.NoError(t, db.SaveRunSetReport("set_c", []byte(`{"status":"pending-approval"}`)))
	assert.Error(t, db.ApproveRunSetPlan("set_c", "changed", []byte(`{"status":"running"}`)))
	record, err := db.GetRunSetApproval("set_c")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "pending", record.Status)
	report, err := db.GetRunSetReport("set_c")
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.JSONEq(t, `{"status":"pending-approval"}`, string(report.Report))

	require.NoError(t, db.ApproveRunSetPlan("set_c", "digest_c", []byte(`{"status":"running","approval_status":"approved"}`)))
	record, err = db.GetRunSetApproval("set_c")
	require.NoError(t, err)
	assert.Equal(t, "approved", record.Status)
	report, err = db.GetRunSetReport("set_c")
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"running","approval_status":"approved"}`, string(report.Report))
}

func TestConcurrentRunSetApprovalsHaveExactlyOneWinner(t *testing.T) {
	first := runsTestDB(t)
	second, err := NewCatalogDB(first.configDir)
	require.NoError(t, err)
	defer second.Close()
	require.NoError(t, first.SaveRunSetApproval("set_race", "digest", []byte(`{"models":["app"]}`), []byte(`{"plan_version":1}`)))
	require.NoError(t, first.SaveRunSetReport("set_race", []byte(`{"status":"pending-approval"}`)))

	start := make(chan struct{})
	errors := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index, db := range []*CatalogDB{first, second} {
		go func(index int, db *CatalogDB) {
			ready.Done()
			<-start
			errors <- db.ApproveRunSetPlan("set_race", "digest", []byte(`{"status":"running","winner":`+string(rune('1'+index))+`}`))
		}(index, db)
	}
	ready.Wait()
	close(start)
	var successes int
	for range 2 {
		if approveErr := <-errors; approveErr == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)

	approval, err := first.GetRunSetApproval("set_race")
	require.NoError(t, err)
	require.NotNil(t, approval)
	assert.Equal(t, "approved", approval.Status)
	report, err := first.GetRunSetReport("set_race")
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Contains(t, []string{`{"status":"running","winner":1}`, `{"status":"running","winner":2}`}, string(report.Report))
}

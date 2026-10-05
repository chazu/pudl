package database

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPreviousSnapshotIsEligibleScopedAndTiesUseInsertionOrder(t *testing.T) {
	db := snapshotTestDB(t)
	at := time.Now().Add(-time.Hour)
	for _, item := range []struct{ id, workspace, status, source string }{{"z_first", "repo", RunStatusSucceeded, SnapshotSourceMuObserve}, {"other", "elsewhere", RunStatusSucceeded, SnapshotSourceMuObserve}, {"failed", "repo", RunStatusFailed, SnapshotSourceMuObserve}, {"registration", "repo", RunStatusSucceeded, SnapshotSourceModelInstance}, {"a_current", "repo", "", SnapshotSourceMuObserve}} {
		snapshot := snapshotFor(item.id, "m", item.source, at)
		snapshot.Workspace = item.workspace
		require.NoError(t, db.RecordObserveSnapshot(snapshot))
		if item.status != "" {
			finishSnapshotRun(t, db, snapshot, item.status)
		}
	}
	previous, pruned, err := db.PreviousSuccessfulObserveSnapshot(context.Background(), "a_current")
	require.NoError(t, err)
	require.False(t, pruned)
	require.NotNil(t, previous)
	require.Equal(t, "z_first", previous.SnapshotID)
}
func TestPreviousSnapshotReportsPrunedInsteadOfSkippingToOlder(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	at := time.Now().Add(-time.Hour)
	for i, id := range []string{"oldest", "previous", "current"} {
		snapshot := snapshotFor(id, "m", SnapshotSourceMuObserve, at.Add(time.Duration(i)*time.Minute))
		seedSnapshot(t, db, snapshot, raw, id+"item")
		finishSnapshotRun(t, db, snapshot, RunStatusSucceeded)
	}
	result, err := db.PruneObserveSnapshots(PruneOptions{Model: "m", Keep: 1, DataDir: dataDir})
	require.NoError(t, err)
	require.Contains(t, result.Snapshots, "previous")
	previous, pruned, err := db.PreviousSuccessfulObserveSnapshot(context.Background(), "current")
	require.NoError(t, err)
	require.True(t, pruned)
	require.Equal(t, "previous", previous.SnapshotID)
}

func TestPreviousSnapshotLegacyCatalogWithoutTombstones(t *testing.T) {
	db := snapshotTestDB(t)
	at := time.Now().Add(-time.Hour)
	previous := snapshotFor("before", "m", SnapshotSourceMuObserve, at)
	current := snapshotFor("now", "m", SnapshotSourceMuObserve, at.Add(time.Minute))
	require.NoError(t, db.RecordObserveSnapshot(previous))
	finishSnapshotRun(t, db, previous, RunStatusSucceeded)
	require.NoError(t, db.RecordObserveSnapshot(current))
	_, err := db.db.Exec("DROP TABLE observe_snapshot_tombstones")
	require.NoError(t, err)
	got, pruned, err := db.PreviousSuccessfulObserveSnapshot(context.Background(), "now")
	require.NoError(t, err)
	require.False(t, pruned)
	require.Equal(t, "before", got.SnapshotID)
}

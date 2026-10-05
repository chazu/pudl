package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotPinsHaveIndependentOwners(t *testing.T) {
	db := snapshotTestDB(t)
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap", "m", SnapshotSourceMuObserve, time.Now())))
	require.NoError(t, db.RetainObserveSnapshot("snap", true))
	for _, id := range []string{"set_a", "set_b"} {
		require.NoError(t, db.SetSnapshotPin(SnapshotPin{SnapshotID: "snap", OwnerKind: SnapshotPinApproval, OwnerID: id}, true))
	}
	require.NoError(t, db.SetSnapshotPin(SnapshotPin{SnapshotID: "snap", OwnerKind: SnapshotPinApproval, OwnerID: "set_a"}, false))
	require.NoError(t, db.RetainObserveSnapshot("snap", false))
	pins, err := db.SnapshotPins("snap")
	require.NoError(t, err)
	require.Len(t, pins, 1)
	require.Equal(t, "set_b", pins[0].OwnerID)
	snapshot, err := db.GetObserveSnapshot("snap")
	require.NoError(t, err)
	require.True(t, snapshot.Retained)
	require.NoError(t, db.SetSnapshotPin(pins[0], false))
	snapshot, err = db.GetObserveSnapshot("snap")
	require.NoError(t, err)
	require.False(t, snapshot.Retained)
}

func TestReportPinsExpireButReferencesSurvivePruning(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	seedSnapshot(t, db, snapshotFor("snap", "m", SnapshotSourceMuObserve, time.Now().Add(-time.Hour)), raw, "item")
	payload := []byte(`{"populate":{"snapshot_id":"snap"},"previous":{"snapshot_id":"snap"}}`)
	require.NoError(t, db.SaveRunReport("run_report", "m", payload))
	result, err := db.PruneObserveSnapshots(PruneOptions{Model: "m", Keep: 0, DataDir: dataDir})
	require.NoError(t, err)
	require.Empty(t, result.Snapshots)
	evidence, err := db.RunReportEvidence("run_report")
	require.NoError(t, err)
	require.Equal(t, []ReportEvidence{{SnapshotID: "snap", Status: "available"}}, evidence)
	_, err = db.db.Exec(`UPDATE snapshot_pins SET expires_at=? WHERE owner_kind=?`, time.Now().Add(-time.Hour), SnapshotPinReport)
	require.NoError(t, err)
	result, err = db.PruneObserveSnapshots(PruneOptions{Model: "m", Keep: 0, DataDir: dataDir})
	require.NoError(t, err)
	require.Equal(t, []string{"snap"}, result.Snapshots)
	evidence, err = db.RunReportEvidence("run_report")
	require.NoError(t, err)
	require.Equal(t, []ReportEvidence{{SnapshotID: "snap", Status: "pruned"}}, evidence)
	stored, err := db.GetRunReport("run_report")
	require.NoError(t, err)
	require.Equal(t, payload, stored.Report)
	var tombstone string
	require.NoError(t, db.db.QueryRow(`SELECT snapshot_id FROM observe_snapshot_tombstones WHERE snapshot_id='snap'`).Scan(&tombstone))
}

func TestPruneProtectsLatestSuccessfulSnapshotDespiteNewFailure(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	for i, id := range []string{"older", "successful", "failed"} {
		run := "run_" + id
		require.NoError(t, db.StartRun(run, "m", "observe-only"))
		status := RunStatusSucceeded
		if id == "failed" {
			status = RunStatusFailed
		}
		require.NoError(t, db.FinishRun(run, RunConclusion{CompletionStatus: status}))
		snapshot := snapshotFor(id, "m", SnapshotSourceMuObserve, time.Now().Add(time.Duration(i-3)*time.Hour))
		snapshot.RunID = run
		snapshot.Workspace = "repo"
		seedSnapshot(t, db, snapshot, raw, id+"_item")
	}
	result, err := db.PruneObserveSnapshots(PruneOptions{Model: "m", Keep: 0, DataDir: dataDir})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"older", "failed"}, result.Snapshots)
	snapshot, err := db.GetObserveSnapshot("successful")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
}

func TestPinMigrationPreservesLegacyRetention(t *testing.T) {
	db := snapshotTestDB(t)
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap", "m", SnapshotSourceMuObserve, time.Now())))
	_, err := db.db.Exec(`UPDATE observe_snapshots SET retained=1 WHERE snapshot_id='snap'`)
	require.NoError(t, err)
	require.NoError(t, db.ensureSnapshotPins())
	pins, err := db.SnapshotPins("snap")
	require.NoError(t, err)
	require.Equal(t, []SnapshotPin{{SnapshotID: "snap", OwnerKind: SnapshotPinManual, OwnerID: "manual"}}, pins)
}

func TestExplicitDeleteCannotBypassEvidenceRetention(t *testing.T) {
	db, _, raw := pruneFixture(t)
	seedSnapshot(t, db, snapshotFor("snap", "m", SnapshotSourceMuObserve, time.Now()), raw, "item")
	require.NoError(t, db.SetSnapshotPin(SnapshotPin{SnapshotID: "snap", OwnerKind: SnapshotPinApproval, OwnerID: "set_a"}, true))
	for _, id := range []string{"snap", "item"} {
		_, err := db.DeleteEntriesAtomic(id, true)
		require.ErrorContains(t, err, "protected snapshot evidence")
	}
	members, err := db.SnapshotRecordEntries("snap")
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.NoError(t, db.SetSnapshotPin(SnapshotPin{SnapshotID: "snap", OwnerKind: SnapshotPinApproval, OwnerID: "set_a"}, false))
	_, err = db.DeleteEntriesAtomic("snap", true)
	require.NoError(t, err)
}

func TestExplicitDeleteCannotEraseLatestSuccessfulBaseline(t *testing.T) {
	db, _, raw := pruneFixture(t)
	require.NoError(t, db.StartRun("run", "m", "observe-only"))
	require.NoError(t, db.FinishRun("run", RunConclusion{CompletionStatus: RunStatusSucceeded}))
	snapshot := snapshotFor("snap", "m", SnapshotSourceMuObserve, time.Now())
	snapshot.RunID = "run"
	snapshot.Workspace = "repo"
	seedSnapshot(t, db, snapshot, raw, "item")
	_, err := db.DeleteEntriesAtomic("snap", true)
	require.ErrorContains(t, err, "protected snapshot evidence")
	_, err = db.DeleteEntriesAtomic("item", false)
	require.ErrorContains(t, err, "protected snapshot evidence")
}

func TestReadOnlyLegacySnapshotInspectionDoesNotRequirePinMigration(t *testing.T) {
	db := snapshotTestDB(t)
	root := db.Root()
	snapshot := snapshotFor("legacy", "m", SnapshotSourceMuObserve, time.Now())
	snapshot.Retained = true
	require.NoError(t, db.RecordObserveSnapshot(snapshot))
	_, err := db.db.Exec(`DROP TABLE snapshot_pins`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	read, err := OpenCatalogDBReadOnly(root)
	require.NoError(t, err)
	defer read.Close()
	got, err := read.GetObserveSnapshot("legacy")
	require.NoError(t, err)
	require.True(t, got.Retained)
	pins, err := read.SnapshotPins("legacy")
	require.NoError(t, err)
	require.Len(t, pins, 1)
	require.Equal(t, "legacy", pins[0].OwnerID)
	snapshots, err := read.ListObserveSnapshots("m", 0)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.False(t, read.hasSnapshotPins())
}

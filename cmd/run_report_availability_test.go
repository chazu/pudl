package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
)

func TestRunReportEvidenceAvailabilityKeepsExactFrozenValues(t *testing.T) {
	root := consolidationWorkspace(t)
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	for _, id := range []string{"previous", "current"} {
		require.NoError(t, db.RecordObserveSnapshot(database.ObserveSnapshot{SnapshotID: id, Model: "m", Workspace: "repo", Source: database.SnapshotSourceMuObserve, CreatedAt: time.Now().Add(-time.Hour)}))
		collection, entryType := "collection", "observe"
		require.NoError(t, db.AddEntry(database.CatalogEntry{ID: id, ImportTimestamp: time.Now().Add(-time.Hour), Format: "json", Schema: "pudl/mu.#ObserveSnapshot", CollectionType: &collection, EntryType: &entryType}))
	}
	require.NoError(t, db.RetainObserveSnapshot("current", true))
	payload := []byte(`{"run_id":"r","model":"m","report_version":1,"drift":{"clean":false,"verified":false,"snapshot_id":"current","drifted":[{"resource":"x","reason":"changed","fields":[{"path":"count","expected":9007199254740993,"observed":1,"previous":{"status":"available","snapshot_id":"previous","value":9007199254740993}}]}]},"future_field":{"exact":9007199254740995}}`)
	require.NoError(t, db.SaveRunReport("r", "m", payload))
	capture := func() string {
		return captureQueryOutput(t, func() error { return runReportCmd.RunE(runReportCmd, []string{"r"}) })
	}
	available := capture()
	require.Contains(t, available, `"evidence_availability"`)
	var document struct {
		Availability []database.ReportEvidence `json:"evidence_availability"`
	}
	require.NoError(t, json.Unmarshal([]byte(available), &document))
	require.Len(t, document.Availability, 2)
	for _, ref := range document.Availability {
		require.Equal(t, "available", ref.Status)
	}
	_, err = db.DB().Exec(`UPDATE snapshot_pins SET expires_at=? WHERE owner_kind=?`, time.Now().Add(-time.Hour), database.SnapshotPinReport)
	require.NoError(t, err)
	pruned, err := db.PruneObserveSnapshots(database.PruneOptions{Model: "m", Keep: 0})
	require.NoError(t, err)
	require.Equal(t, []string{"previous"}, pruned.Snapshots)
	after := capture()
	require.NoError(t, json.Unmarshal([]byte(after), &document))
	require.Equal(t, []database.ReportEvidence{{SnapshotID: "current", Status: "available"}, {SnapshotID: "previous", Status: "pruned"}}, document.Availability)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(after), &raw))
	require.JSONEq(t, `{"exact":9007199254740995}`, string(raw["future_field"]))
	require.Contains(t, string(raw["drift"]), "9007199254740993")
	require.Contains(t, string(raw["drift"]), `"status": "available"`, "historical previous status is frozen; availability is a separate current annotation")
	stored, err := db.GetRunReport("r")
	require.NoError(t, err)
	require.Equal(t, payload, stored.Report)
	jsonOutput = false
	human := capture()
	require.Contains(t, human, "current: available")
	require.Contains(t, human, "previous: pruned")
	require.Contains(t, human, "9007199254740993")
}

func TestRunSetReportAggregatesMemberEvidenceAndPreservesUnknownFields(t *testing.T) {
	root := consolidationWorkspace(t)
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SaveRunReport("member-run", "m", []byte(`{"run_id":"member-run","populate":{"snapshot_id":"gone"}}`)))
	require.NoError(t, db.SaveRunSetReport("set", []byte(`{"run_set_id":"set","status":"succeeded","members":[{"model":"m","run_id":"member-run","result":"succeeded"}],"future_field":9007199254740993}`)))
	capture := func() string {
		return captureQueryOutput(t, func() error { return runReportCmd.RunE(runReportCmd, []string{"set"}) })
	}
	output := capture()
	var document struct {
		Availability []operationEvidence `json:"evidence_availability"`
		Future       json.RawMessage     `json:"future_field"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &document))
	require.Len(t, document.Availability, 1)
	require.Equal(t, "member-run", document.Availability[0].RunID)
	require.Equal(t, "gone", document.Availability[0].SnapshotID)
	require.Equal(t, "pruned", document.Availability[0].Status)
	require.Equal(t, "9007199254740993", string(document.Future))
	jsonOutput = false
	human := capture()
	require.Contains(t, human, "gone: pruned (run member-run)")
}

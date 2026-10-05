package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/mubridge"
	"github.com/stretchr/testify/require"
)

func TestInventoryPreviousEvidenceFrozenAcrossNewObservationsAndReplay(t *testing.T) {
	db, dataDir := inventoryFixture(t)
	ingest := func(id, number string) {
		runID := "run_" + id
		require.NoError(t, db.StartRun(runID, "m", "observe-only"))
		_, err := mubridge.IngestObserve(db, mubridge.ObserveIngest{Reader: strings.NewReader(`[{"target":"//m:observe","current":{"name":"x","count":` + number + `}}]`), DataDir: dataDir, SnapshotID: id, Model: "m", RunID: runID, Workspace: "repo", Origin: "pudl-run", Source: database.SnapshotSourceMuObserve})
		require.NoError(t, err)
		require.NoError(t, db.FinishRun(runID, database.RunConclusion{CompletionStatus: database.RunStatusSucceeded}))
	}
	ingest("previous", "9007199254740993")
	ingest("current", "1")
	desired := []map[string]any{{"name": "x", "count": json.Number("9007199254740993")}}
	result, err := runInventoryDrift(db, "current", desired, nil)
	require.NoError(t, err)
	require.Len(t, result.Drifted, 1)
	require.Len(t, result.Drifted[0].Fields, 1)
	previous := result.Drifted[0].Fields[0].Previous
	require.Equal(t, "available", previous.Status)
	require.Equal(t, "previous", previous.SnapshotID)
	require.Equal(t, "9007199254740993", string(previous.Value))
	report := RunReport{RunID: "reported", Model: "m", Drift: &result}
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	require.NoError(t, db.SaveRunReport("reported", "m", encoded))
	ingest("later", "2")
	stored, err := db.GetRunReport("reported")
	require.NoError(t, err)
	require.Equal(t, encoded, stored.Report)
	var replay RunReport
	decoder := json.NewDecoder(strings.NewReader(string(stored.Report)))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&replay))
	require.Equal(t, "9007199254740993", string(replay.Drift.Drifted[0].Fields[0].Previous.Value))
}

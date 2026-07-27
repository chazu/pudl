package mubridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

func driftTestDB(t *testing.T) (*database.CatalogDB, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "pudl-drift-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	db, err := database.NewCatalogDB(filepath.Join(dir, "db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, filepath.Join(dir, "data")
}

func TestRecordDriftObservationStoresEvidence(t *testing.T) {
	db, dataDir := driftTestDB(t)

	id, err := RecordDriftObservation(db, DriftObservation{
		Target:       "models/app:drift",
		RunID:        "run_abc",
		Clean:        false,
		DriftedCount: 1,
		Drifted:      []map[string]any{{"resource": "Deployment/web", "reason": "missing"}},
		Raw:          json.RawMessage(`[{"target":"//models/app:drift"}]`),
	}, dataDir)

	require.NoError(t, err)
	require.NotEmpty(t, id)

	entry, err := db.GetEntry(id)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.NotNil(t, entry.EntryType)
	assert.Equal(t, DriftObservationEntryType, *entry.EntryType)
	assert.Equal(t, DriftObservationSchema, entry.Schema)
	require.NotNil(t, entry.RunID)
	assert.Equal(t, "run_abc", *entry.RunID)
	require.NotNil(t, entry.Target)
	assert.Equal(t, "models/app:drift", *entry.Target)

	// The verdict must be re-derivable from what was stored.
	raw, err := os.ReadFile(entry.StoredPath)
	require.NoError(t, err)
	var stored DriftObservation
	require.NoError(t, json.Unmarshal(raw, &stored))
	assert.False(t, stored.Clean)
	assert.Equal(t, 1, stored.DriftedCount)
	assert.Contains(t, string(stored.Raw), "models/app:drift")
}

// The entry type is deliberately not "observe": inventory drift loads observed
// records as entry_type=observe + collection_type=item, and a drift verdict must
// never be mistaken for an observed resource.
func TestRecordDriftObservationDoesNotPolluteObserveQueries(t *testing.T) {
	db, dataDir := driftTestDB(t)

	canned := `[{"target":"//host:odroid","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"}
	]}}]`
	_, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(canned), Origin: "pudl-run", DataDir: dataDir, Graph: nil})
	require.NoError(t, err)

	_, err = RecordDriftObservation(db, DriftObservation{
		Target: "models/app:drift",
		Clean:  true,
	}, dataDir)
	require.NoError(t, err)

	observed, err := db.QueryEntries(database.FilterOptions{
		EntryTypes:     []string{"observe"},
		CollectionType: "item",
	}, database.QueryOptions{})
	require.NoError(t, err)

	for _, entry := range observed.Entries {
		assert.NotEqual(t, DriftObservationSchema, entry.Schema,
			"a drift observation must not appear among observed records")
	}
	assert.Equal(t, 1, len(observed.Entries), "only the ingested package record")
}

// Identical observations are the same observation; a re-record reuses the entry
// rather than minting a duplicate.
func TestRecordDriftObservationDedupesIdenticalContent(t *testing.T) {
	db, dataDir := driftTestDB(t)

	observation := DriftObservation{
		ObservationID: "drift_fixed",
		Timestamp:     "2026-07-24T00:00:00Z",
		Target:        "models/app:drift",
		Clean:         true,
	}

	first, err := RecordDriftObservation(db, observation, dataDir)
	require.NoError(t, err)
	second, err := RecordDriftObservation(db, observation, dataDir)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

func TestRecordDriftObservationRejectsEmptyTarget(t *testing.T) {
	db, dataDir := driftTestDB(t)

	_, err := RecordDriftObservation(db, DriftObservation{Clean: true}, dataDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty target")
}

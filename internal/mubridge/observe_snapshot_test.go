package mubridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const snapshotTestInput = `[{"target":"//app","current":{"records":[
	{"_schema":"linux.host","hostname":"box"},
	{"_schema":"linux.host","hostname":"box2"}
]}}]`

func TestIngestObserve_RecordsTheSnapshotContract(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	result, err := IngestObserve(db, ObserveIngest{
		Reader:     strings.NewReader(snapshotTestInput),
		DataDir:    dataDir,
		SnapshotID: "snap_fixed",
		RunID:      "run_a",
		Model:      "hostinv",
		Workspace:  "myrepo",
		Origin:     "pudl-run",
		Source:     database.SnapshotSourceMuObserve,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Records)
	assert.Equal(t, "snap_fixed", result.SnapshotID)

	snapshot, err := db.GetObserveSnapshot("snap_fixed")
	require.NoError(t, err)
	require.NotNil(t, snapshot, "the contract is queryable, not buried in a JSON blob on disk")
	assert.Equal(t, "run_a", snapshot.RunID)
	assert.Equal(t, "hostinv", snapshot.Model)
	assert.Equal(t, "myrepo", snapshot.Workspace)
	assert.Equal(t, "pudl-run", snapshot.Origin)
	assert.Equal(t, database.SnapshotSourceMuObserve, snapshot.Source)
	assert.Equal(t, []string{"app"}, snapshot.Targets)
	assert.Equal(t, 2, snapshot.RecordCount)
}

func TestIngestObserve_SnapshotIDIsTheCollectionEntryID(t *testing.T) {
	// One identifier, not two. The collection entry used to be keyed on the hash
	// of its own payload while a readable id sat unused inside it.
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir,
		SnapshotID: "snap_fixed", Model: "hostinv",
	})
	require.NoError(t, err)

	entry, err := db.GetCollectionByID(result.SnapshotID)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "snap_fixed", entry.ID)
	require.NotNil(t, entry.ContentHash)
	assert.NotEqual(t, entry.ID, *entry.ContentHash, "the content hash is retained where it belongs")

	records, err := db.SnapshotRecordEntries("snap_fixed")
	require.NoError(t, err)
	assert.Len(t, records, 2)
}

func TestIngestObserve_GeneratesAnIDWhenNoRunAllocatedOne(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir,
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(result.SnapshotID, "snap_"))

	snapshot, err := db.GetObserveSnapshot(result.SnapshotID)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, database.SnapshotSourceIngestObserve, snapshot.Source, "the default source")
	assert.Empty(t, snapshot.Model, "a standalone ingest is not taken on behalf of a model")
}

func TestIngestObserve_FailedIngestLeavesNoSnapshot(t *testing.T) {
	// D6's actual guarantee. A mid-ingest failure used to leave an orphan
	// snapshot row the run never learned the id of — the inverse of the property
	// the decision claimed. The transaction is what provides it; this asserts it
	// end to end, including the new contract row.
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// Make the day's raw directory unwritable so staging the snapshot's evidence
	// file fails after the transaction has begun.
	now := time.Now()
	rawDir := filepath.Join(dataDir, "raw", now.Format("2006"), now.Format("01"), now.Format("02"))
	require.NoError(t, os.MkdirAll(rawDir, 0o755))
	require.NoError(t, os.Chmod(rawDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(rawDir, 0o755) })

	_, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir,
		SnapshotID: "snap_doomed", RunID: "run_a", Model: "hostinv",
	})
	require.Error(t, err)

	snapshot, err := db.GetObserveSnapshot("snap_doomed")
	require.NoError(t, err)
	assert.Nil(t, snapshot, "no contract row for an observation that never completed")

	entry, err := db.GetCollectionByID("snap_doomed")
	assert.Error(t, err, "and no collection entry either")
	assert.Nil(t, entry)
}

func TestIngestObserve_TwoRunsGetTwoSnapshots(t *testing.T) {
	// Snapshot dedup stays rejected: a snapshot is the record of one observation
	// by one run, which is what --catalog-scope selects.
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	first, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir,
		SnapshotID: "snap_1", RunID: "run_1", Model: "m",
	})
	require.NoError(t, err)
	second, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir,
		SnapshotID: "snap_2", RunID: "run_2", Model: "m",
	})
	require.NoError(t, err)

	assert.NotEqual(t, first.SnapshotID, second.SnapshotID)

	current, err := db.CurrentObserveSnapshot("m")
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, "snap_2", current.SnapshotID)

	// The shared records belong to both snapshots — membership is the
	// many-to-many relationship, so nothing is duplicated and nothing is stolen.
	records, err := db.SnapshotRecordEntries("snap_1")
	require.NoError(t, err)
	require.Len(t, records, 2)
	count, err := db.ItemMembershipCount(records[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// Two observations of the same target within one second must not overwrite each
// other's evidence.
//
// The raw filename used to be a function of (second, target, index), so a
// converge loop re-observing — or two models watching one host — wrote both
// records to the same path. The catalog entries stayed distinct, so the first
// snapshot went on pointing at a file that now held the second observation's
// record, and an inventory set-diff against that snapshot could report clean off
// data it never observed.
func TestIngestObserve_SameSecondObservationsKeepSeparateEvidence(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	before := `[{"target":"//host:box","current":{"records":[{"_schema":"linux.host","hostname":"box","kernel":"6.0"}]}}]`
	after := `[{"target":"//host:box","current":{"records":[{"_schema":"linux.host","hostname":"box","kernel":"6.1"}]}}]`

	require.NoError(t, ingestOne(db, dataDir, "snap_before", before))
	require.NoError(t, ingestOne(db, dataDir, "snap_after", after))

	first, err := db.SnapshotRecordEntries("snap_before")
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, err := db.SnapshotRecordEntries("snap_after")
	require.NoError(t, err)
	require.Len(t, second, 1)

	require.NotEqual(t, first[0].StoredPath, second[0].StoredPath,
		"distinct records must not share a stored path")

	firstBytes, err := os.ReadFile(first[0].StoredPath)
	require.NoError(t, err)
	assert.Contains(t, string(firstBytes), `"6.0"`,
		"the earlier snapshot still points at what it actually observed")

	secondBytes, err := os.ReadFile(second[0].StoredPath)
	require.NoError(t, err)
	assert.Contains(t, string(secondBytes), `"6.1"`)
}

func ingestOne(db *database.CatalogDB, dataDir, snapshotID, observeJSON string) error {
	_, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(observeJSON), DataDir: dataDir,
		SnapshotID: snapshotID, Model: "m", Source: database.SnapshotSourceMuObserve,
	})
	return err
}

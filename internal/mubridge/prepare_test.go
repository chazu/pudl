package mubridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/stretchr/testify/require"
)

func TestObservationPreparationDoesNotAcquireSQLWriter(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	conn, err := db.DB().Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE")
	require.NoError(t, err)
	prepared, err := PrepareObservation(ObserveIngest{Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir, SnapshotID: "snap_prepared"})
	require.NoError(t, err)
	defer prepared.Close()
	rows, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	require.Empty(t, rows.Entries)
	_, err = conn.ExecContext(context.Background(), "ROLLBACK")
	require.NoError(t, err)
	result, err := prepared.Commit(db)
	require.NoError(t, err)
	require.Equal(t, 2, result.Records)
}

func TestObservationLimitsAndMalformedInputPublishNothing(t *testing.T) {
	for _, limits := range []ingestprep.Limits{{RecordBytes: 10}, {DecodedBytes: 10}, {StagingBytes: 1}} {
		db, dataDir := setupIngestTestDB(t)
		_, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir, Limits: limits})
		require.Error(t, err)
		rows, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
		require.NoError(t, err)
		require.Empty(t, rows.Entries)
		db.Close()
		entries, err := os.ReadDir(filepath.Join(dataDir, "tmp"))
		require.NoError(t, err)
		require.Empty(t, entries)
	}
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	_, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(snapshotTestInput + " true"), DataDir: dataDir})
	require.Error(t, err)
	rows, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	require.Empty(t, rows.Entries)
}

func TestObservationCommitFailureDoesNotRemoveExistingEvidence(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	_, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir, SnapshotID: "snap_original"})
	require.NoError(t, err)
	records, err := db.SnapshotRecordEntries("snap_original")
	require.NoError(t, err)
	_, err = db.DB().Exec(`CREATE TRIGGER deny_prepared BEFORE INSERT ON observe_snapshots WHEN NEW.snapshot_id='snap_fail' BEGIN SELECT RAISE(ABORT,'fail'); END`)
	require.NoError(t, err)
	_, err = IngestObserve(db, ObserveIngest{Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir, SnapshotID: "snap_fail"})
	require.Error(t, err)
	for _, entry := range records {
		_, err := os.Stat(entry.StoredPath)
		require.NoError(t, err)
	}
	snapshot, err := db.GetObserveSnapshot("snap_fail")
	require.NoError(t, err)
	require.Nil(t, snapshot)
}

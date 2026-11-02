package observe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sharedRecord is byte-identical whichever collector reports it, so both
// collectors' records hash to the same content-addressed entry ID.
const sharedRecord = `{"_schema":"linux.host","hostname":"shared","kernel":"6.0"}`

func sharedRecordInput(target string) string {
	return `[{"target":"//` + target + `","current":{"records":[` + sharedRecord + `]}}]`
}

// Two models whose collectors produce a byte-identical record must share one
// entry: the second ingest reuses it and records it as a member of its own
// snapshot. It used to fail with "UNIQUE constraint failed: catalog_entries.id"
// because the dedup lookup was scoped to the collector's target while the entry
// ID is the content hash alone.
func TestIngestObserve_SecondCollectorReusesByteIdenticalRecord(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	first, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(sharedRecordInput("collector/a")), DataDir: dataDir,
		SnapshotID: "snap_a", Model: "model_a", Scope: "model_a", Complete: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, first.Records)

	second, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(sharedRecordInput("collector/b")), DataDir: dataDir,
		SnapshotID: "snap_b", Model: "model_b", Scope: "model_b", Complete: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, second.Records, "the record is observed in the second snapshot")
	assert.Equal(t, 0, second.NewRecords, "the record is deduplicated, not stored twice")

	aRecords, err := db.SnapshotRecordEntries("snap_a")
	require.NoError(t, err)
	require.Len(t, aRecords, 1)
	bRecords, err := db.SnapshotRecordEntries("snap_b")
	require.NoError(t, err)
	require.Len(t, bRecords, 1, "the second model's snapshot must contain the shared record")
	assert.Equal(t, aRecords[0].ID, bRecords[0].ID)

	count, err := db.ItemMembershipCount(aRecords[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// The same holds within one observation: two targets reporting the same record
// share one entry, which the snapshot holds once.
func TestIngestObserve_ByteIdenticalRecordsAcrossTargetsInOneBatch(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[
		{"target":"//collector/a","current":{"records":[` + sharedRecord + `]}},
		{"target":"//collector/b","current":{"records":[` + sharedRecord + `]}}
	]`
	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), DataDir: dataDir, SnapshotID: "snap_ab",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Records)

	records, err := db.SnapshotRecordEntries("snap_ab")
	require.NoError(t, err)
	require.Len(t, records, 1)
}

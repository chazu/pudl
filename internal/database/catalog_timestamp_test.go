package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTimestampRoundTrip guards against the modernc/sqlite failure mode where
// binding a raw time.Time (especially one parsed into a fixed-offset zone)
// stores Go's Time.String() output (e.g. "... -0400 -0400"), which the driver
// then cannot scan back into time.Time. AddEntry + UpdateEntry must store
// timestamps in a form that scans cleanly on the next read.
func TestTimestampRoundTrip(t *testing.T) {
	db, err := NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	// A fixed-offset timestamp like one parsed from stored text — the value that
	// triggered the corruption when bound as a raw time.Time.
	imported := time.Date(2026, 4, 23, 9, 49, 21, 370318000, time.FixedZone("", -4*3600))

	entry := CatalogEntry{
		ID:              "ts-roundtrip-1",
		StoredPath:      "/tmp/x.json",
		ImportTimestamp: imported,
		Format:          "json",
		Origin:          "test",
		Schema:          "pudl/core.#Item",
		Confidence:      1.0,
	}

	require.NoError(t, db.AddEntry(entry))

	// Reading back must not fail on the timestamp scan.
	res, err := db.QueryEntries(FilterOptions{}, QueryOptions{})
	require.NoError(t, err, "query after AddEntry should scan timestamps cleanly")
	require.Len(t, res.Entries, 1)
	require.True(t, res.Entries[0].ImportTimestamp.Equal(imported),
		"import_timestamp not preserved: got %v want %v", res.Entries[0].ImportTimestamp, imported)

	// UpdateEntry re-binds import_timestamp/updated_at; the round-tripped value
	// must still scan (this is the exact path the identity migration exercised).
	updated := res.Entries[0]
	require.NoError(t, db.UpdateEntry(updated))

	res2, err := db.QueryEntries(FilterOptions{}, QueryOptions{})
	require.NoError(t, err, "query after UpdateEntry should scan timestamps cleanly")
	require.Len(t, res2.Entries, 1)
	require.True(t, res2.Entries[0].ImportTimestamp.Equal(imported),
		"import_timestamp not preserved after update: got %v want %v", res2.Entries[0].ImportTimestamp, imported)
}

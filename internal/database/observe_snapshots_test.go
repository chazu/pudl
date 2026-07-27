package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotTestDB(t *testing.T) *CatalogDB {
	t.Helper()
	dir := t.TempDir()
	db, err := NewCatalogDB(dir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// seedSnapshot writes a snapshot contract row, its collection entry, and one
// record per name — the shape the ingest produces.
func seedSnapshot(t *testing.T, db *CatalogDB, snapshot ObserveSnapshot, rawDir string, recordNames ...string) {
	t.Helper()
	require.NoError(t, db.RecordObserveSnapshot(snapshot))

	entryType := "observe"
	collectionType := "collection"
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID:              snapshot.SnapshotID,
		ImportTimestamp: snapshot.CreatedAt,
		Format:          "json",
		Origin:          snapshot.Origin,
		Schema:          "pudl/mu.#ObserveSnapshot",
		EntryType:       &entryType,
		CollectionType:  &collectionType,
	}))

	itemType := "item"
	for i, name := range recordNames {
		path := filepath.Join(rawDir, name+".json")
		require.NoError(t, os.MkdirAll(rawDir, 0o755))
		require.NoError(t, os.WriteFile(path, []byte(`{"name":"`+name+`"}`), 0o644))

		// Content-addressed: the same record name is the same entry, shared
		// between snapshots, exactly as the ingest's dedup produces.
		if existing, err := db.GetEntry(name); err == nil && existing != nil {
			require.NoError(t, db.AddCollectionMembership(snapshot.SnapshotID, name, i))
			continue
		}
		require.NoError(t, db.AddEntry(CatalogEntry{
			ID:              name,
			StoredPath:      path,
			ImportTimestamp: snapshot.CreatedAt,
			Format:          "json",
			Origin:          snapshot.Origin,
			Schema:          "test.#Resource",
			EntryType:       &entryType,
			CollectionType:  &itemType,
		}))
		require.NoError(t, db.AddCollectionMembership(snapshot.SnapshotID, name, i))
	}
}

func snapshotFor(id, model, source string, at time.Time) ObserveSnapshot {
	return ObserveSnapshot{
		SnapshotID: id, Model: model, Source: source, RunID: "run_" + id,
		Workspace: "repo", Origin: "pudl-run", Targets: []string{"models/" + model},
		RecordCount: 1, CreatedAt: at,
	}
}

func TestObserveSnapshot_ContractRoundTrips(t *testing.T) {
	db := snapshotTestDB(t)
	created := time.Now().Truncate(time.Millisecond)

	require.NoError(t, db.RecordObserveSnapshot(ObserveSnapshot{
		SnapshotID: "snap_a", RunID: "run_a", Model: "m", Workspace: "repo",
		Origin: "pudl-run", Source: SnapshotSourceMuObserve,
		Targets:     []string{"models/m:populate", "models/m:extra"},
		RecordCount: 7, CreatedAt: created,
	}))

	got, err := db.GetObserveSnapshot("snap_a")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run_a", got.RunID)
	assert.Equal(t, "m", got.Model)
	assert.Equal(t, "repo", got.Workspace)
	assert.Equal(t, "pudl-run", got.Origin)
	assert.Equal(t, SnapshotSourceMuObserve, got.Source)
	assert.Equal(t, []string{"models/m:populate", "models/m:extra"}, got.Targets)
	assert.Equal(t, 7, got.RecordCount)
	assert.False(t, got.Retained)
	assert.WithinDuration(t, created, got.CreatedAt, time.Second)
}

func TestObserveSnapshot_MissingIsNilNotAnError(t *testing.T) {
	// A snapshot recorded before this table existed is still a valid replay
	// scope; it simply has no contract row.
	db := snapshotTestDB(t)
	got, err := db.GetObserveSnapshot("snap_from_before")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCurrentObserveSnapshot_NewestForThatModel(t *testing.T) {
	db := snapshotTestDB(t)
	base := time.Now().Add(-time.Hour)

	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_old", "m", SnapshotSourceMuObserve, base)))
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_new", "m", SnapshotSourceMuObserve, base.Add(30*time.Minute))))
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_other", "other", SnapshotSourceMuObserve, base.Add(59*time.Minute))))

	current, err := db.CurrentObserveSnapshot("m")
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, "snap_new", current.SnapshotID, "another model's newer snapshot is not this model's current one")
}

func TestCurrentObserveSnapshot_IgnoresModelInstanceRegistrations(t *testing.T) {
	// The model-instance row reuses the observe ingester, but it is a
	// registration, not an observation of the live system. If it answered
	// "current", every run would shadow its own populate snapshot with the row it
	// writes first.
	db := snapshotTestDB(t)
	base := time.Now().Add(-time.Hour)

	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_observed", "m", SnapshotSourceMuObserve, base)))
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_registered", "m", SnapshotSourceModelInstance, base.Add(time.Minute))))

	current, err := db.CurrentObserveSnapshot("m")
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, "snap_observed", current.SnapshotID)
}

func TestCurrentObserveSnapshot_NoneIsNil(t *testing.T) {
	db := snapshotTestDB(t)
	current, err := db.CurrentObserveSnapshot("never-observed")
	require.NoError(t, err)
	assert.Nil(t, current)
}

func TestSnapshotRecordEntries_ReturnsOnlyThatSnapshotsMembers(t *testing.T) {
	db := snapshotTestDB(t)
	raw := filepath.Join(t.TempDir(), "raw")
	now := time.Now()

	seedSnapshot(t, db, snapshotFor("snap_a", "m", SnapshotSourceMuObserve, now), raw, "rec1", "rec2")
	seedSnapshot(t, db, snapshotFor("snap_b", "other", SnapshotSourceMuObserve, now), raw, "rec3")

	entries, err := db.SnapshotRecordEntries("snap_a")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "rec1", entries[0].ID)
	assert.Equal(t, "rec2", entries[1].ID)
}

func TestRetainObserveSnapshot(t *testing.T) {
	db := snapshotTestDB(t)
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_a", "m", SnapshotSourceMuObserve, time.Now())))

	require.NoError(t, db.RetainObserveSnapshot("snap_a", true))
	got, err := db.GetObserveSnapshot("snap_a")
	require.NoError(t, err)
	assert.True(t, got.Retained)

	require.NoError(t, db.RetainObserveSnapshot("snap_a", false))
	got, err = db.GetObserveSnapshot("snap_a")
	require.NoError(t, err)
	assert.False(t, got.Retained)

	assert.Error(t, db.RetainObserveSnapshot("snap_nonexistent", true))
}

func TestListObserveSnapshots_NewestFirstAndFiltered(t *testing.T) {
	db := snapshotTestDB(t)
	base := time.Now().Add(-time.Hour)

	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_1", "m", SnapshotSourceMuObserve, base)))
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_2", "m", SnapshotSourceMuObserve, base.Add(time.Minute))))
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("snap_3", "other", SnapshotSourceMuObserve, base.Add(2*time.Minute))))

	all, err := db.ListObserveSnapshots("", 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "snap_3", all[0].SnapshotID)

	mine, err := db.ListObserveSnapshots("m", 0)
	require.NoError(t, err)
	require.Len(t, mine, 2)
	assert.Equal(t, "snap_2", mine[0].SnapshotID)

	limited, err := db.ListObserveSnapshots("", 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}

package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pruneFixture returns a db plus the data dir whose raw/ tree bounds file
// removal, matching how the real ingest stages records.
func pruneFixture(t *testing.T) (*CatalogDB, string, string) {
	t.Helper()
	db := snapshotTestDB(t)
	dataDir := t.TempDir()
	return db, dataDir, filepath.Join(dataDir, "raw")
}

func TestPrune_KeepsTheNewestN(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)
	for i, id := range []string{"snap_1", "snap_2", "snap_3", "snap_4"} {
		seedSnapshot(t, db, snapshotFor(id, "m", SnapshotSourceMuObserve, base.Add(time.Duration(i)*time.Hour)), raw, id+"_rec")
	}

	result, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 2, OlderThan: time.Now(), DataDir: dataDir,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"snap_1", "snap_2"}, result.Snapshots)

	remaining, err := db.ListObserveSnapshots("m", 0)
	require.NoError(t, err)
	require.Len(t, remaining, 2)
	assert.Equal(t, "snap_4", remaining[0].SnapshotID)
}

func TestPrune_NeverTakesTheCurrentSnapshot(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)
	for i, id := range []string{"snap_1", "snap_2"} {
		seedSnapshot(t, db, snapshotFor(id, "m", SnapshotSourceMuObserve, base.Add(time.Duration(i)*time.Hour)), raw, id+"_rec")
	}

	_, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 1, OlderThan: time.Now(), DataDir: dataDir,
	})
	require.NoError(t, err)

	current, err := db.CurrentObserveSnapshot("m")
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, "snap_2", current.SnapshotID)
}

func TestPrune_RequiresBothConditions(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)
	for i, id := range []string{"snap_1", "snap_2", "snap_3"} {
		seedSnapshot(t, db, snapshotFor(id, "m", SnapshotSourceMuObserve, base.Add(time.Duration(i)*time.Hour)), raw, id+"_rec")
	}

	// Keep alone, with no age cutoff, still prunes: the age condition is optional.
	// What must not happen is the reverse — an age cutoff wiping a model's whole
	// history because Keep defaulted to zero.
	byAgeOnly, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 0, OlderThan: time.Now().Add(-24 * time.Hour), DataDir: dataDir, DryRun: true,
	})
	require.NoError(t, err)
	assert.Empty(t, byAgeOnly.Snapshots, "nothing is older than the cutoff")

	byKeepOnly, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 1, DataDir: dataDir, DryRun: true,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"snap_1", "snap_2"}, byKeepOnly.Snapshots)
}

func TestPrune_RespectsRetention(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)
	for i, id := range []string{"snap_1", "snap_2", "snap_3"} {
		seedSnapshot(t, db, snapshotFor(id, "m", SnapshotSourceMuObserve, base.Add(time.Duration(i)*time.Hour)), raw, id+"_rec")
	}
	require.NoError(t, db.RetainObserveSnapshot("snap_1", true))

	result, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 1, OlderThan: time.Now(), DataDir: dataDir,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap_2"}, result.Snapshots)

	pinned, err := db.GetObserveSnapshot("snap_1")
	require.NoError(t, err)
	require.NotNil(t, pinned)
}

func TestPrune_LeavesRecordsStillCitedByASurvivingSnapshot(t *testing.T) {
	// Records are content-addressed and deliberately shared between snapshots.
	// Deleting a pruned snapshot's records outright would silently empty whichever
	// other snapshots still cite them.
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)

	seedSnapshot(t, db, snapshotFor("snap_1", "m", SnapshotSourceMuObserve, base), raw, "shared", "only_old")
	seedSnapshot(t, db, snapshotFor("snap_2", "m", SnapshotSourceMuObserve, base.Add(time.Hour)), raw, "shared")

	result, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 1, OlderThan: time.Now(), DataDir: dataDir,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap_1"}, result.Snapshots)
	assert.Equal(t, 1, result.Records, "only the record no snapshot still cites")

	shared, err := db.GetEntry("shared")
	require.NoError(t, err)
	require.NotNil(t, shared, "the shared record survives with its remaining snapshot")

	survivors, err := db.SnapshotRecordEntries("snap_2")
	require.NoError(t, err)
	require.Len(t, survivors, 1)
	assert.Equal(t, "shared", survivors[0].ID)

	_, err = db.GetEntry("only_old")
	assert.Error(t, err, "the record nothing cites is gone")
}

func TestPrune_UnlinksRawFilesOnlyUnderTheDataDir(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)
	seedSnapshot(t, db, snapshotFor("snap_1", "m", SnapshotSourceMuObserve, base), raw, "rec_inside")
	seedSnapshot(t, db, snapshotFor("snap_2", "m", SnapshotSourceMuObserve, base.Add(time.Hour)), raw, "rec_keep")

	// A record staged outside the data dir must be left alone and reported.
	outside := filepath.Join(t.TempDir(), "elsewhere.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{}`), 0o644))
	entryType, itemType := "observe", "item"
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID: "rec_outside", StoredPath: outside, ImportTimestamp: base, Format: "json",
		Origin: "pudl-run", Schema: "test.#Resource", EntryType: &entryType, CollectionType: &itemType,
	}))
	require.NoError(t, db.AddCollectionMembership("snap_1", "rec_outside", 1))

	inside := filepath.Join(raw, "rec_inside.json")
	require.FileExists(t, inside)

	result, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 1, OlderThan: time.Now(), DataDir: dataDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.FilesRemoved)
	assert.Equal(t, []string{outside}, result.FilesSkipped)
	assert.NoFileExists(t, inside)
	assert.FileExists(t, outside, "a path outside the data dir's raw/ tree is never unlinked")
}

func TestPrune_IgnoresSnapshotsWithNoContractRow(t *testing.T) {
	// Snapshots recorded before the contract existed carry no model and no
	// retention flag, so no policy can be evaluated against them. Deleting what
	// cannot be evaluated is not a policy.
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-100 * time.Hour)

	entryType, collectionType := "observe", "collection"
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID: "legacy_hash", ImportTimestamp: base, Format: "json", Origin: "mu-observe",
		Schema: "pudl/mu.#ObserveSnapshot", EntryType: &entryType, CollectionType: &collectionType,
	}))
	seedSnapshot(t, db, snapshotFor("snap_1", "m", SnapshotSourceMuObserve, base.Add(time.Hour)), raw, "rec")

	result, err := db.PruneObserveSnapshots(PruneOptions{
		Keep: 0, OlderThan: time.Now(), DataDir: dataDir,
	})
	require.NoError(t, err)
	assert.NotContains(t, result.Snapshots, "legacy_hash")

	legacy, err := db.GetEntry("legacy_hash")
	require.NoError(t, err)
	require.NotNil(t, legacy)
}

func TestPrune_DryRunChangesNothing(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)
	seedSnapshot(t, db, snapshotFor("snap_1", "m", SnapshotSourceMuObserve, base), raw, "rec_1")
	seedSnapshot(t, db, snapshotFor("snap_2", "m", SnapshotSourceMuObserve, base.Add(time.Hour)), raw, "rec_2")

	result, err := db.PruneObserveSnapshots(PruneOptions{
		Model: "m", Keep: 1, OlderThan: time.Now(), DataDir: dataDir, DryRun: true,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap_1"}, result.Snapshots)
	assert.Equal(t, 1, result.Records, "a dry run reports the record impact, not only snapshot ids")
	assert.Equal(t, 1, result.FilesRemoved)

	still, err := db.GetObserveSnapshot("snap_1")
	require.NoError(t, err)
	require.NotNil(t, still, "nothing was removed")
	assert.FileExists(t, filepath.Join(raw, "rec_1.json"))
}

func TestPrune_KeepIsPerModel(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	base := time.Now().Add(-10 * time.Hour)
	seedSnapshot(t, db, snapshotFor("snap_a1", "a", SnapshotSourceMuObserve, base), raw, "a1")
	seedSnapshot(t, db, snapshotFor("snap_a2", "a", SnapshotSourceMuObserve, base.Add(time.Hour)), raw, "a2")
	seedSnapshot(t, db, snapshotFor("snap_b1", "b", SnapshotSourceMuObserve, base.Add(2*time.Hour)), raw, "b1")

	result, err := db.PruneObserveSnapshots(PruneOptions{
		Keep: 1, OlderThan: time.Now(), DataDir: dataDir,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap_a1"}, result.Snapshots,
		"model b's single snapshot is its newest, so Keep 1 protects it")
}

func TestPrunableRawFile(t *testing.T) {
	assert.True(t, prunableRawFile("/data/raw/2026/07/x.json", "/data"))
	assert.False(t, prunableRawFile("/data/other/x.json", "/data"))
	assert.False(t, prunableRawFile("/elsewhere/x.json", "/data"))
	assert.False(t, prunableRawFile("", "/data"))
	assert.False(t, prunableRawFile("/data/raw/x.json", ""), "no data dir means no file removal")
	assert.False(t, prunableRawFile("/data/rawsomething/x.json", "/data"),
		"a sibling directory sharing the prefix is not inside raw/")
}

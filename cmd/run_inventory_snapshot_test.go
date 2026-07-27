package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/mubridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ingestSnapshot records one observation under an explicit snapshot id, the way
// a run's populate phase does.
func ingestSnapshot(t *testing.T, db *database.CatalogDB, dataDir, snapshotID, model, observeJSON string) {
	t.Helper()
	_, err := mubridge.IngestObserve(db, mubridge.ObserveIngest{
		Reader:     strings.NewReader(observeJSON),
		DataDir:    dataDir,
		SnapshotID: snapshotID,
		Model:      model,
		Origin:     "pudl-run",
		Source:     database.SnapshotSourceMuObserve,
	})
	require.NoError(t, err)
}

func inventoryFixture(t *testing.T) (*database.CatalogDB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := database.NewCatalogDB(filepath.Join(dir, "db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, filepath.Join(dir, "data")
}

// A stale snapshot must not decide a later run's verdict. Both snapshots sit in
// the same catalog under the same origin; only the snapshot id separates them,
// which is the whole reason the run compares against the snapshot it just
// populated rather than "every observe record".
func TestInventoryDrift_StaleSnapshotCannotSatisfyANewerRun(t *testing.T) {
	db, dataDir := inventoryFixture(t)

	ingestSnapshot(t, db, dataDir, "snap_old", "hostinv", `[{"target":"//host:box","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"},
		{"_schema":"pudl/linux.#Package","name":"restic","state":"present"}
	]}}]`)
	// The package was removed by the time of the second observation.
	ingestSnapshot(t, db, dataDir, "snap_new", "hostinv", `[{"target":"//host:box","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"}
	]}}]`)

	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"},
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "present"},
	}

	fresh, err := runInventoryDrift(db, "snap_new", desired, nil)
	require.NoError(t, err)
	assert.False(t, fresh.Clean, "restic is gone, and the stale snapshot must not say otherwise")
	require.Len(t, fresh.Drifted, 1)
	assert.Equal(t, "missing", fresh.Drifted[0].Reason)

	// The old snapshot still describes the old world — replaying it is a
	// legitimate, explicitly requested operation, and it is `Verified: false`
	// for exactly that reason.
	stale, err := runInventoryDrift(db, "snap_old", desired, nil)
	require.NoError(t, err)
	assert.True(t, stale.Clean)
	assert.False(t, stale.Verified)
}

// Another model's snapshot must not satisfy this model's desired records, even
// when the records are identical and share an origin.
func TestInventoryDrift_AnotherModelsSnapshotCannotSatisfyThisOne(t *testing.T) {
	db, dataDir := inventoryFixture(t)

	records := `[{"target":"//host:box","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"}
	]}}]`
	ingestSnapshot(t, db, dataDir, "snap_theirs", "othermodel", records)

	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"},
	}

	// This model has observed nothing. Scoped to its own (absent) snapshot the
	// desired record is missing; scoped to theirs it is satisfied — which is why
	// the scope must be the run's own snapshot and never an unscoped query.
	mine, err := runInventoryDrift(db, "snap_mine_never_taken", desired, nil)
	require.NoError(t, err)
	assert.False(t, mine.Clean)

	theirs, err := runInventoryDrift(db, "snap_theirs", desired, nil)
	require.NoError(t, err)
	assert.True(t, theirs.Clean)
}

// A snapshot id resolves as a collection scope, not as an origin. If it fell
// through to the origin filter it would match every record ingested under
// "pudl-run" — every model, every snapshot.
func TestInventoryDrift_SnapshotScopeIsNotAnOriginScope(t *testing.T) {
	db, dataDir := inventoryFixture(t)

	ingestSnapshot(t, db, dataDir, "snap_a", "m", `[{"target":"//host:box","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"}
	]}}]`)
	ingestSnapshot(t, db, dataDir, "snap_b", "m", `[{"target":"//host:box","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"restic","state":"present"}
	]}}]`)

	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "present"},
	}

	scoped, err := runInventoryDrift(db, "snap_a", desired, nil)
	require.NoError(t, err)
	assert.False(t, scoped.Clean, "snap_a does not contain restic, whatever else shares its origin")

	byOrigin, err := runInventoryDrift(db, "pudl-run", desired, nil)
	require.NoError(t, err)
	assert.True(t, byOrigin.Clean, "the origin scope deliberately spans both snapshots")
}

// Snapshots recorded before the snapshot contract existed are keyed on the hash
// of their payload. They must keep resolving as replay scopes.
func TestInventoryDrift_LegacyContentHashScopeStillResolves(t *testing.T) {
	db, dataDir := inventoryFixture(t)

	entryType, collectionType, itemType := "observe", "collection", "item"
	legacyID := "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: legacyID, ImportTimestamp: time.Now(), Format: "json", Origin: "mu-observe",
		Schema: "pudl/mu.#ObserveSnapshot", EntryType: &entryType, CollectionType: &collectionType,
	}))

	// One record inside it, staged the way the old ingest did.
	recordPath := filepath.Join(dataDir, "legacy_record.json")
	require.NoError(t, writeFileEnsuringDir(recordPath,
		`{"_schema":"pudl/linux.#Package","name":"podman","state":"present"}`))
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: "legacy_record", StoredPath: recordPath, ImportTimestamp: time.Now(), Format: "json",
		Origin: "mu-observe", Schema: "pudl/linux.#Package",
		EntryType: &entryType, CollectionType: &itemType, CollectionID: &legacyID,
	}))
	require.NoError(t, db.AddCollectionMembership(legacyID, "legacy_record", 0))

	// No contract row exists for it, which must not stop it being a valid scope.
	snapshot, err := db.GetObserveSnapshot(legacyID)
	require.NoError(t, err)
	require.Nil(t, snapshot)

	res, err := runInventoryDrift(db, legacyID, []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"},
	}, nil)
	require.NoError(t, err)
	assert.True(t, res.Clean)
}

// writeFileEnsuringDir stages a raw record file, creating its directory.
func writeFileEnsuringDir(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

package database

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func migrationVersions(t *testing.T, db *CatalogDB) []int {
	t.Helper()
	versions, err := db.AppliedMigrationVersions()
	require.NoError(t, err)
	return versions
}

func TestMigrations_FreshDatabaseRecordsEveryVersion(t *testing.T) {
	db := snapshotTestDB(t)

	want := make([]int, 0, len(migrations))
	for _, m := range migrations {
		want = append(want, m.version)
	}
	assert.Equal(t, want, migrationVersions(t, db))
}

func TestMigrations_SecondOpenReRunsNothing(t *testing.T) {
	dir := t.TempDir()
	first, err := NewCatalogDB(dir)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer second.Close()

	// A migration that re-ran would rewrite applied_at. Capture the ledger before
	// and after: identical rows mean nothing was applied twice.
	var appliedAt string
	require.NoError(t, second.db.QueryRow(
		`SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&appliedAt))

	third, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer third.Close()

	var appliedAtAgain string
	require.NoError(t, third.db.QueryRow(
		`SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&appliedAtAgain))
	assert.Equal(t, appliedAt, appliedAtAgain, "an already-recorded migration must not re-run")
}

func TestMigrations_UnrecordedMigrationIsApplied(t *testing.T) {
	// The transition case: a database that predates the ledger has no version
	// rows, so every migration is unapplied and runs. Simulated by clearing the
	// ledger and reopening.
	dir := t.TempDir()
	first, err := NewCatalogDB(dir)
	require.NoError(t, err)
	_, err = first.db.Exec(`DELETE FROM schema_migrations`)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer second.Close()

	assert.Len(t, migrationVersions(t, second), len(migrations),
		"every migration re-runs and is recorded, which is safe because each is idempotent")
}

func TestMigrations_FailureIsNotRecorded(t *testing.T) {
	// A migration that errors must stay unrecorded so the next open retries it,
	// and nothing after it may run: a later migration can assume its shape.
	db := snapshotTestDB(t)
	_, err := db.db.Exec(`DELETE FROM schema_migrations`)
	require.NoError(t, err)

	failing := migration{version: 9998, name: "boom", apply: func(*CatalogDB) error {
		return errors.New("deliberate migration failure")
	}}
	after := migration{version: 9999, name: "after", apply: func(c *CatalogDB) error {
		_, err := c.db.Exec(`CREATE TABLE should_not_exist (x INTEGER)`)
		return err
	}}

	original := migrations
	migrations = append(append([]migration{}, original...), failing, after)
	t.Cleanup(func() { migrations = original })

	err = db.runMigrations()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration 9998 (boom)")

	applied, err := db.appliedMigrations()
	require.NoError(t, err)
	assert.False(t, applied[9998], "a failed migration is not recorded")
	assert.False(t, applied[9999], "and nothing after it runs")

	var count int
	require.NoError(t, db.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='should_not_exist'`).Scan(&count))
	assert.Equal(t, 0, count)
}

func TestMigrations_ViewsAreRecreatedEveryOpen(t *testing.T) {
	// Views are not versioned: they restate the Go source that declares them, so
	// a change to a view body must take effect without anyone bumping a number.
	dir := t.TempDir()
	first, err := NewCatalogDB(dir)
	require.NoError(t, err)
	_, err = first.db.Exec(`DROP VIEW ` + CatalogEntryView)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer second.Close()

	var count int
	require.NoError(t, second.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='view' AND name=?`, CatalogEntryView).Scan(&count))
	assert.Equal(t, 1, count, "the view is rebuilt even though every migration was already recorded")
}

func TestMigrations_LegacyCollectionColumnsAreRetired(t *testing.T) {
	db := snapshotTestDB(t)

	for _, column := range []string{"collection_id", "item_index"} {
		exists, err := db.columnExists("catalog_entries", column)
		require.NoError(t, err)
		assert.False(t, exists, "%s must be gone: collection_memberships is the sole relationship source", column)
	}
}

func TestMigrations_LegacyDatabaseIsBackfilledThenRetired(t *testing.T) {
	// A database old enough to still carry the columns must have its memberships
	// backfilled from them before they are dropped — migration 5 then 12.
	db := snapshotTestDB(t)

	// Recreate the legacy shape and a row that only the column describes.
	_, err := db.db.Exec(`ALTER TABLE catalog_entries ADD COLUMN collection_id TEXT`)
	require.NoError(t, err)
	_, err = db.db.Exec(`ALTER TABLE catalog_entries ADD COLUMN item_index INTEGER`)
	require.NoError(t, err)

	entryType, itemType, collectionType := "observe", "item", "collection"
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID: "legacy_collection", Format: "json", Origin: "test", Schema: "test.#C",
		EntryType: &entryType, CollectionType: &collectionType,
	}))
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID: "legacy_item", Format: "json", Origin: "test", Schema: "test.#R",
		EntryType: &entryType, CollectionType: &itemType,
	}))
	// Written straight to the column, the way the old insert did, with no
	// membership row.
	_, err = db.db.Exec(
		`UPDATE catalog_entries SET collection_id = 'legacy_collection', item_index = 0 WHERE id = 'legacy_item'`)
	require.NoError(t, err)
	_, err = db.db.Exec(`DELETE FROM collection_memberships WHERE item_id = 'legacy_item'`)
	require.NoError(t, err)
	_, err = db.db.Exec(`DELETE FROM schema_migrations WHERE version IN (5, 12)`)
	require.NoError(t, err)

	require.NoError(t, db.runMigrations())

	items, err := db.GetCollectionItems("legacy_collection")
	require.NoError(t, err)
	require.Len(t, items, 1, "the legacy column was backfilled into a membership")
	assert.Equal(t, "legacy_item", items[0].ID)

	exists, err := db.columnExists("catalog_entries", "collection_id")
	require.NoError(t, err)
	assert.False(t, exists, "and then dropped")
}

package database

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func schemaVersion(t *testing.T, db *CatalogDB) int {
	t.Helper()
	var v int
	require.NoError(t, db.db.QueryRow(`PRAGMA schema_version`).Scan(&v))
	return v
}

func recordedViewHash(t *testing.T, db *CatalogDB) string {
	t.Helper()
	var h string
	require.NoError(t, db.db.QueryRow(
		`SELECT value FROM catalog_meta WHERE key = ?`, catalogEntryViewMetaKey).Scan(&h))
	return h
}

func TestCatalogEntryView_UnchangedOpenWritesNoSchema(t *testing.T) {
	dir := t.TempDir()
	first, err := NewCatalogDB(dir)
	require.NoError(t, err)
	before := schemaVersion(t, first)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(catalogEntryViewSQL))), recordedViewHash(t, first))
	require.NoError(t, first.Close())

	second, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer second.Close()
	assert.Equal(t, before, schemaVersion(t, second),
		"reopening with an unchanged view definition must not drop or recreate it")
}

func TestCatalogEntryView_ChangedDefinitionIsRebuilt(t *testing.T) {
	dir := t.TempDir()
	first, err := NewCatalogDB(dir)
	require.NoError(t, err)
	// Stand in for a database built from an older view definition.
	_, err = first.db.Exec(`UPDATE catalog_meta SET value = 'stale' WHERE key = ?`, catalogEntryViewMetaKey)
	require.NoError(t, err)
	before := schemaVersion(t, first)
	require.NoError(t, first.Close())

	second, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer second.Close()
	assert.Greater(t, schemaVersion(t, second), before, "a changed definition is rebuilt")
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(catalogEntryViewSQL))), recordedViewHash(t, second))

	var views int
	require.NoError(t, second.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='view' AND name=?`, CatalogEntryView).Scan(&views))
	assert.Equal(t, 1, views)
}

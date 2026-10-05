package lister

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestFailedDeleteLeavesPayloadAndMetadataIntact(t *testing.T) {
	root := t.TempDir()
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	raw := filepath.Join(root, "data", "raw", "item.json")
	meta := filepath.Join(root, "data", "metadata", "item.meta")
	for _, path := range []string{raw, meta} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	}
	require.NoError(t, db.AddEntry(database.CatalogEntry{ID: "item", StoredPath: raw, MetadataPath: meta, ImportTimestamp: time.Now(), Format: "json", Schema: "pudl/core.#Item"}))
	require.NoError(t, db.Close())
	sqlDB, err := sql.Open("sqlite", filepath.Join(root, "data", "sqlite", "catalog.db"))
	require.NoError(t, err)
	defer sqlDB.Close()
	_, err = sqlDB.Exec(`CREATE TRIGGER reject_delete BEFORE DELETE ON catalog_entries BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	require.NoError(t, err)
	l, err := New(filepath.Join(root, "data"))
	require.NoError(t, err)
	defer l.Close()
	result, err := l.DeleteEntry("item", false)
	require.Error(t, err)
	require.Nil(t, result)
	require.FileExists(t, raw)
	require.FileExists(t, meta)
	_, err = l.catalogDB.GetEntry("item")
	require.NoError(t, err)
}

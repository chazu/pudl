package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeleteEntriesRollbackPreservesMembershipAndFiles(t *testing.T) {
	root := t.TempDir()
	db, err := NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	raw := filepath.Join(root, "data", "raw", "item.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(raw), 0755))
	require.NoError(t, os.WriteFile(raw, []byte(`{}`), 0600))
	collection, item := "collection", "item"
	for _, entry := range []CatalogEntry{{ID: "c", CollectionType: &collection}, {ID: "i", StoredPath: raw, CollectionType: &item}} {
		entry.ImportTimestamp = time.Now()
		entry.Format = "json"
		entry.Schema = "pudl/core.#Item"
		require.NoError(t, db.AddEntry(entry))
	}
	require.NoError(t, db.AddCollectionMembership("c", "i", 0))
	_, err = db.db.Exec(`CREATE TRIGGER reject_delete BEFORE DELETE ON catalog_entries BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	require.NoError(t, err)
	_, err = db.DeleteEntriesAtomic("c", true)
	require.Error(t, err)
	_, err = db.GetEntry("i")
	require.NoError(t, err)
	count, err := db.ItemMembershipCount("i")
	require.NoError(t, err)
	require.Equal(t, 1, count)
	_, err = os.Stat(raw)
	require.NoError(t, err)
}

func TestRemoveCommittedOrphanPreservesReferencedFile(t *testing.T) {
	root := t.TempDir()
	db, err := NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	path := filepath.Join(root, "data", "raw", "shared.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	require.NoError(t, db.AddEntry(CatalogEntry{ID: "shared", StoredPath: path, ImportTimestamp: time.Now(), Format: "json", Schema: "pudl/core.#Item"}))
	removed, err := db.RemoveCommittedOrphan(path)
	require.NoError(t, err)
	require.False(t, removed)
	require.NoError(t, db.DeleteEntry("shared"))
	removed, err = db.RemoveCommittedOrphan(path)
	require.NoError(t, err)
	require.True(t, removed)
}

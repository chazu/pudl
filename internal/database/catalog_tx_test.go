package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func txTestEntry(id, target string) CatalogEntry {
	t := target
	entryType := "manifest-action"
	return CatalogEntry{
		ID: id, StoredPath: id + ".json", MetadataPath: id + ".meta",
		ImportTimestamp: time.Now(), Format: "json", Origin: "t",
		Schema: "pudl/core.#Item", Target: &t, EntryType: &entryType,
	}
}

func TestWithCatalogTx_CommitsOnSuccess(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	require.NoError(t, db.WithCatalogTx(func(tx *CatalogTx) error {
		if err := tx.AddEntry(txTestEntry("a", "web")); err != nil {
			return err
		}
		return tx.UpdateStatus("web", "converging")
	}))

	entry, err := db.GetEntry("a")
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.NotNil(t, entry.Status)
	assert.Equal(t, "converging", *entry.Status)
}

// The whole point: a step that fails part-way leaves nothing behind, rather than
// the prefix of itself that happened to succeed.
func TestWithCatalogTx_RollsBackOnError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	wanted := fmt.Errorf("step failed after some writes landed")
	err := db.WithCatalogTx(func(tx *CatalogTx) error {
		if err := tx.AddEntry(txTestEntry("a", "web")); err != nil {
			return err
		}
		if err := tx.UpdateStatus("web", "converging"); err != nil {
			return err
		}
		return wanted
	})
	require.ErrorIs(t, err, wanted)

	_, err = db.GetEntry("a")
	require.Error(t, err, "the entry written before the failure must not survive")

	statuses, err := db.GetTargetStatuses()
	require.NoError(t, err)
	assert.Empty(t, statuses, "nor the status written before it")
}

// A transaction must read its own uncommitted writes, or every check-then-write
// inside a step (dedup, status repair) silently stops working.
func TestCatalogTx_ReadsSeeUncommittedWrites(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	require.NoError(t, db.WithCatalogTx(func(tx *CatalogTx) error {
		if err := tx.AddEntry(txTestEntry("a", "web")); err != nil {
			return err
		}

		entry, err := tx.GetEntry("a")
		if err != nil {
			return fmt.Errorf("entry written in this transaction is not visible to it: %w", err)
		}
		assert.Equal(t, "a", entry.ID)

		if err := tx.UpdateStatus("web", "converging"); err != nil {
			return err
		}
		statuses, err := tx.GetTargetStatuses()
		if err != nil {
			return err
		}
		require.Len(t, statuses, 1)
		assert.Equal(t, "converging", statuses[0].Status, "status written in this transaction is visible to it")
		return nil
	}))
}

// Content-hash lookups are how both steps dedup, so they must see the
// transaction's own inserts — a batch containing the same record twice
// deduplicates against itself.
func TestCatalogTx_ContentHashLookupsSeeUncommittedWrites(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	hash := "deadbeef"
	require.NoError(t, db.WithCatalogTx(func(tx *CatalogTx) error {
		entry := txTestEntry("a", "web")
		entry.ContentHash = &hash
		observeType := "observe"
		entry.EntryType = &observeType
		if err := tx.AddEntry(entry); err != nil {
			return err
		}

		found, err := tx.FindByContentHash(hash)
		if err != nil {
			return err
		}
		require.NotNil(t, found, "FindByContentHash must see this transaction's insert")
		assert.Equal(t, "a", found.ID)

		observed, err := tx.GetLatestObserveByContentHash("web", hash)
		if err != nil {
			return err
		}
		require.NotNil(t, observed, "observe dedup must see this transaction's insert")
		assert.Equal(t, "a", observed.ID)
		return nil
	}))
}

// A collection item and its membership row are written by the same call, so they
// must roll back together — a membership pointing at a row that does not exist
// is exactly the dangling state the step boundary exists to prevent.
func TestWithCatalogTx_EntryAndMembershipRollBackTogether(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	collectionID := "snap-1"
	itemIndex := 0
	collectionType := "item"

	wanted := fmt.Errorf("step failed")
	err := db.WithCatalogTx(func(tx *CatalogTx) error {
		entry := txTestEntry("item-a", "web")
		entry.CollectionID = &collectionID
		entry.ItemIndex = &itemIndex
		entry.CollectionType = &collectionType
		if err := tx.AddEntry(entry); err != nil {
			return err
		}
		return wanted
	})
	require.ErrorIs(t, err, wanted)

	count, err := db.ItemMembershipCount("item-a")
	require.NoError(t, err)
	assert.Zero(t, count, "the membership rolled back with the entry it belonged to")
}

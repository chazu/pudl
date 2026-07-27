package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// membershipItem adds one collection item, recording its membership.
func membershipItem(t *testing.T, db *CatalogDB, id, collectionID string, index int) {
	t.Helper()
	entryType, itemType := "observe", "item"
	if _, err := db.GetEntry(id); err != nil {
		require.NoError(t, db.AddEntry(CatalogEntry{
			ID: id, Format: "json", Origin: "test", Schema: "test.#R",
			EntryType: &entryType, CollectionType: &itemType,
			CollectionID: &collectionID, ItemIndex: &index,
		}))
		return
	}
	require.NoError(t, db.AddCollectionMembership(collectionID, id, index))
}

func membershipCollection(t *testing.T, db *CatalogDB, id string) {
	t.Helper()
	entryType, collectionType := "observe", "collection"
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID: id, Format: "json", Origin: "test", Schema: "test.#C",
		EntryType: &entryType, CollectionType: &collectionType,
	}))
}

func TestMembership_UnscopedReadReportsTheSoleCollection(t *testing.T) {
	db := snapshotTestDB(t)
	membershipCollection(t, db, "coll_a")
	membershipItem(t, db, "item_1", "coll_a", 3)

	entry, err := db.GetEntry("item_1")
	require.NoError(t, err)
	require.NotNil(t, entry.CollectionID)
	assert.Equal(t, "coll_a", *entry.CollectionID)
	require.NotNil(t, entry.ItemIndex)
	assert.Equal(t, 3, *entry.ItemIndex)
}

func TestMembership_UnscopedReadOfASharedItemReportsNoCollection(t *testing.T) {
	// An item in several collections has no single collection ID. The legacy
	// column returned one anyway — whichever inserted it first — so a record
	// observed by three snapshots claimed to belong only to the oldest.
	db := snapshotTestDB(t)
	membershipCollection(t, db, "coll_a")
	membershipCollection(t, db, "coll_b")
	membershipItem(t, db, "item_1", "coll_a", 0)
	membershipItem(t, db, "item_1", "coll_b", 1)

	entry, err := db.GetEntry("item_1")
	require.NoError(t, err)
	assert.Nil(t, entry.CollectionID, "no single answer, so no answer")
	assert.Nil(t, entry.ItemIndex)
}

func TestMembership_CollectionScopedReadCarriesThatCollection(t *testing.T) {
	// The one reading that is well-defined for a shared item: read *through* a
	// collection and you get that collection and the index within it.
	db := snapshotTestDB(t)
	membershipCollection(t, db, "coll_a")
	membershipCollection(t, db, "coll_b")
	membershipItem(t, db, "item_1", "coll_a", 0)
	membershipItem(t, db, "item_1", "coll_b", 7)

	viaA, err := db.GetCollectionItems("coll_a")
	require.NoError(t, err)
	require.Len(t, viaA, 1)
	require.NotNil(t, viaA[0].CollectionID)
	assert.Equal(t, "coll_a", *viaA[0].CollectionID)
	assert.Equal(t, 0, *viaA[0].ItemIndex)

	viaB, err := db.GetCollectionItems("coll_b")
	require.NoError(t, err)
	require.Len(t, viaB, 1)
	assert.Equal(t, "coll_b", *viaB[0].CollectionID)
	assert.Equal(t, 7, *viaB[0].ItemIndex)
}

func TestMembership_RemovingAMembershipStopsTheClaim(t *testing.T) {
	// The legacy column could not be corrected: removing a membership left it
	// naming a collection the item was no longer in.
	db := snapshotTestDB(t)
	membershipCollection(t, db, "coll_a")
	membershipItem(t, db, "item_1", "coll_a", 0)

	require.NoError(t, db.RemoveCollectionMembership("coll_a", "item_1"))

	entry, err := db.GetEntry("item_1")
	require.NoError(t, err)
	assert.Nil(t, entry.CollectionID)
}

func TestMembership_SharedItemLosingOneMembershipReportsTheRemainingOne(t *testing.T) {
	db := snapshotTestDB(t)
	membershipCollection(t, db, "coll_a")
	membershipCollection(t, db, "coll_b")
	membershipItem(t, db, "item_1", "coll_a", 0)
	membershipItem(t, db, "item_1", "coll_b", 1)

	require.NoError(t, db.RemoveCollectionMembership("coll_a", "item_1"))

	entry, err := db.GetEntry("item_1")
	require.NoError(t, err)
	require.NotNil(t, entry.CollectionID)
	assert.Equal(t, "coll_b", *entry.CollectionID, "one membership left, so it is unambiguous again")
	assert.Equal(t, 1, *entry.ItemIndex)
}

func TestMembership_QueryEntriesScopedByCollection(t *testing.T) {
	db := snapshotTestDB(t)
	membershipCollection(t, db, "coll_a")
	membershipCollection(t, db, "coll_b")
	membershipItem(t, db, "item_1", "coll_a", 0)
	membershipItem(t, db, "item_2", "coll_b", 0)

	result, err := db.QueryEntries(FilterOptions{CollectionID: "coll_a"}, QueryOptions{})
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "item_1", result.Entries[0].ID)
}

func TestCatalogEntryView_CollectionIDMatchesTheUnambiguousMembership(t *testing.T) {
	// Datalog rules join on catalog_entry.collection_id; the view must expose the
	// membership, not a value written once at insert.
	db := snapshotTestDB(t)
	membershipCollection(t, db, "coll_a")
	membershipCollection(t, db, "coll_b")
	membershipItem(t, db, "item_sole", "coll_a", 0)
	membershipItem(t, db, "item_shared", "coll_a", 1)
	membershipItem(t, db, "item_shared", "coll_b", 0)

	var sole string
	require.NoError(t, db.db.QueryRow(
		`SELECT collection_id FROM `+CatalogEntryView+` WHERE id = 'item_sole'`).Scan(&sole))
	assert.Equal(t, "coll_a", sole)

	var shared *string
	require.NoError(t, db.db.QueryRow(
		`SELECT collection_id FROM `+CatalogEntryView+` WHERE id = 'item_shared'`).Scan(&shared))
	assert.Nil(t, shared, "a shared item exposes no single collection to Datalog either")
}

func TestAddEntry_DefaultsContentHashAndVersionAtInsert(t *testing.T) {
	// These used to be repaired by a backfill on the *next* open, which only
	// worked while every migration re-ran every time.
	db := snapshotTestDB(t)
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID: "entry_1", Format: "json", Origin: "test", Schema: "test.#R",
	}))

	entry, err := db.GetEntry("entry_1")
	require.NoError(t, err)
	require.NotNil(t, entry.ContentHash)
	assert.Equal(t, "entry_1", *entry.ContentHash)
	require.NotNil(t, entry.Version)
	assert.Equal(t, 1, *entry.Version)
}

package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCollectionMembershipsShareContentAddressedItems(t *testing.T) {
	db, err := NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	collectionType := "collection"
	for _, id := range []string{"collection-a", "collection-b"} {
		require.NoError(t, db.AddEntry(CatalogEntry{
			ID:              id,
			StoredPath:      "/tmp/" + id,
			MetadataPath:    "/tmp/" + id + ".meta",
			ImportTimestamp: time.Now(),
			Format:          "ndjson",
			Origin:          id,
			Schema:          "pudl/core.#Collection",
			CollectionType:  &collectionType,
		}))
	}

	itemType := "item"
	itemIndex := 0
	item := CatalogEntry{
		ID:              "shared-item",
		StoredPath:      "/tmp/shared-item",
		MetadataPath:    "/tmp/shared-item.meta",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "collection-a_item_0",
		Schema:          "pudl/core.#Item",
		CollectionID:    stringPtr("collection-a"),
		ItemIndex:       &itemIndex,
		CollectionType:  &itemType,
		ItemID:          stringPtr("shared-item"),
	}
	require.NoError(t, db.AddEntry(item))
	require.NoError(t, db.AddCollectionMembership("collection-b", item.ID, 3))

	itemsA, err := db.GetCollectionItems("collection-a")
	require.NoError(t, err)
	itemsB, err := db.GetCollectionItems("collection-b")
	require.NoError(t, err)
	require.Len(t, itemsA, 1)
	require.Len(t, itemsB, 1)
	require.Equal(t, "collection-a", *itemsA[0].CollectionID)
	require.Equal(t, "collection-b", *itemsB[0].CollectionID)
	require.Equal(t, 3, *itemsB[0].ItemIndex)

	require.NoError(t, db.DeleteEntry("collection-a"))
	remaining, err := db.GetCollectionItems("collection-b")
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	itemExists, err := db.EntryExists(item.ID)
	require.NoError(t, err)
	require.True(t, itemExists)
}

func stringPtr(value string) *string { return &value }

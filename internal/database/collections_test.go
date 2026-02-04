package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectionOperations(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	t.Run("create collection with items", func(t *testing.T) {
		collectionType := "collection"
		itemType := "item"
		collectionID := "test-collection-001"

		// Create collection entry
		collection := CatalogEntry{
			ID:              collectionID,
			StoredPath:      "/test/collections/test-collection-001.json",
			MetadataPath:    "/test/metadata/test-collection-001.meta",
			ImportTimestamp: time.Now(),
			Format:          "ndjson",
			Origin:          "test-collection",
			Schema:          "core.#Collection",
			Confidence:      0.95,
			RecordCount:     5,
			SizeBytes:       2500,
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  &collectionType,
			ItemID:          nil,
		}

		err := suite.DB.AddEntry(collection)
		require.NoError(t, err)

		// Create collection items
		items := make([]CatalogEntry, 5)
		for i := 0; i < 5; i++ {
			itemID := fmt.Sprintf("%s-item-%d", collectionID, i)
			itemIndex := i

			items[i] = CatalogEntry{
				ID:              itemID,
				StoredPath:      fmt.Sprintf("/test/items/%s.json", itemID),
				MetadataPath:    fmt.Sprintf("/test/metadata/%s.meta", itemID),
				ImportTimestamp: time.Now().Add(time.Duration(i) * time.Second),
				Format:          "json",
				Origin:          fmt.Sprintf("test-item-%d", i),
				Schema:          "core.#CollectionItem",
				Confidence:      0.8,
				RecordCount:     1,
				SizeBytes:       500,
				CollectionID:    &collectionID,
				ItemIndex:       &itemIndex,
				CollectionType:  &itemType,
				ItemID:          &itemID,
			}

			err := suite.DB.AddEntry(items[i])
			require.NoError(t, err)
		}

		// Verify collection exists
		retrievedCollection, err := suite.DB.GetEntry(collectionID)
		require.NoError(t, err)
		AssertDatabaseEntry(t, &collection, retrievedCollection)

		// Verify all items exist and have correct relationships
		for i, expectedItem := range items {
			retrievedItem, err := suite.DB.GetEntry(expectedItem.ID)
			require.NoError(t, err)
			AssertDatabaseEntry(t, &expectedItem, retrievedItem)

			// Verify collection relationships
			assert.Equal(t, collectionID, *retrievedItem.CollectionID)
			assert.Equal(t, i, *retrievedItem.ItemIndex)
			assert.Equal(t, "item", *retrievedItem.CollectionType)
			assert.Equal(t, expectedItem.ID, *retrievedItem.ItemID)
		}

		// Verify total count
		AssertDatabaseCount(t, suite.DB, 6) // 1 collection + 5 items
	})

	t.Run("query collection items", func(t *testing.T) {
		collectionID := "test-collection-001"

		// Query all items in the collection
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionID: collectionID,
		}, QueryOptions{
			SortBy: "timestamp",
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find all 5 items
		assert.Equal(t, 5, len(result.Entries))

		// All should belong to the collection
		for _, entry := range result.Entries {
			assert.Equal(t, collectionID, *entry.CollectionID)
			assert.Equal(t, "item", *entry.CollectionType)
		}

		// Should be sorted by timestamp
		for i := 1; i < len(result.Entries); i++ {
			assert.True(t, 
				result.Entries[i-1].ImportTimestamp.Before(result.Entries[i].ImportTimestamp) ||
				result.Entries[i-1].ImportTimestamp.Equal(result.Entries[i].ImportTimestamp),
				"Items should be sorted by timestamp")
		}
	})
}

func TestCollectionCascadeOperations(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	collectionType := "collection"
	itemType := "item"
	collectionID := "cascade-test-collection"

	// Create collection with items
	collection := CatalogEntry{
		ID:              collectionID,
		StoredPath:      "/test/collections/cascade-test-collection.json",
		MetadataPath:    "/test/metadata/cascade-test-collection.meta",
		ImportTimestamp: time.Now(),
		Format:          "ndjson",
		Origin:          "cascade-test",
		Schema:          "core.#Collection",
		Confidence:      0.95,
		RecordCount:     3,
		SizeBytes:       1500,
		CollectionID:    nil,
		ItemIndex:       nil,
		CollectionType:  &collectionType,
		ItemID:          nil,
	}

	require.NoError(t, suite.DB.AddEntry(collection))

	// Create items
	itemIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		itemID := fmt.Sprintf("%s-item-%d", collectionID, i)
		itemIDs[i] = itemID
		itemIndex := i

		item := CatalogEntry{
			ID:              itemID,
			StoredPath:      fmt.Sprintf("/test/items/%s.json", itemID),
			MetadataPath:    fmt.Sprintf("/test/metadata/%s.meta", itemID),
			ImportTimestamp: time.Now().Add(time.Duration(i) * time.Second),
			Format:          "json",
			Origin:          fmt.Sprintf("cascade-test-item-%d", i),
			Schema:          "core.#CollectionItem",
			Confidence:      0.8,
			RecordCount:     1,
			SizeBytes:       500,
			CollectionID:    &collectionID,
			ItemIndex:       &itemIndex,
			CollectionType:  &itemType,
			ItemID:          &itemID,
		}

		require.NoError(t, suite.DB.AddEntry(item))
	}

	// Verify initial state
	AssertDatabaseCount(t, suite.DB, 4) // 1 collection + 3 items

	t.Run("delete collection item", func(t *testing.T) {
		// Delete one item
		err := suite.DB.DeleteEntry(itemIDs[1])
		require.NoError(t, err)

		// Verify item is deleted
		AssertEntryNotExists(t, suite.DB, itemIDs[1])

		// Verify collection and other items still exist
		AssertEntryExists(t, suite.DB, collectionID)
		AssertEntryExists(t, suite.DB, itemIDs[0])
		AssertEntryExists(t, suite.DB, itemIDs[2])

		// Verify count
		AssertDatabaseCount(t, suite.DB, 3) // 1 collection + 2 items
	})

	t.Run("update collection", func(t *testing.T) {
		// Update collection metadata
		updatedCollection := collection
		updatedCollection.RecordCount = 2 // Reflect deleted item
		updatedCollection.SizeBytes = 1000
		updatedCollection.Confidence = 0.9

		err := suite.DB.UpdateEntry(updatedCollection)
		require.NoError(t, err)

		// Verify update
		retrievedCollection, err := suite.DB.GetEntry(collectionID)
		require.NoError(t, err)
		assert.Equal(t, 2, retrievedCollection.RecordCount)
		assert.Equal(t, int64(1000), retrievedCollection.SizeBytes)
		assert.Equal(t, 0.9, retrievedCollection.Confidence)
	})

	t.Run("delete collection", func(t *testing.T) {
		// Delete the collection
		err := suite.DB.DeleteEntry(collectionID)
		require.NoError(t, err)

		// Verify collection is deleted
		AssertEntryNotExists(t, suite.DB, collectionID)

		// Note: In this implementation, items are not automatically deleted
		// when collection is deleted (no cascade delete implemented)
		// Items become orphaned but still exist
		AssertEntryExists(t, suite.DB, itemIDs[0])
		AssertEntryExists(t, suite.DB, itemIDs[2])

		// Verify count (2 orphaned items remain)
		AssertDatabaseCount(t, suite.DB, 2)
	})
}

func TestCollectionQueries(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	// Create multiple collections with different characteristics
	collections := []struct {
		id     string
		format string
		schema string
		items  int
	}{
		{"logs-collection", "ndjson", "logs.#LogCollection", 10},
		{"metrics-collection", "json", "metrics.#MetricCollection", 5},
		{"events-collection", "ndjson", "events.#EventCollection", 8},
	}

	for _, coll := range collections {
		collectionType := "collection"
		itemType := "item"

		// Create collection
		collection := CatalogEntry{
			ID:              coll.id,
			StoredPath:      fmt.Sprintf("/test/collections/%s.%s", coll.id, coll.format),
			MetadataPath:    fmt.Sprintf("/test/metadata/%s.meta", coll.id),
			ImportTimestamp: time.Now(),
			Format:          coll.format,
			Origin:          fmt.Sprintf("%s-origin", coll.id),
			Schema:          coll.schema,
			Confidence:      0.95,
			RecordCount:     coll.items,
			SizeBytes:       int64(coll.items * 100),
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  &collectionType,
			ItemID:          nil,
		}

		require.NoError(t, suite.DB.AddEntry(collection))

		// Create items
		for i := 0; i < coll.items; i++ {
			itemID := fmt.Sprintf("%s-item-%d", coll.id, i)
			itemIndex := i

			item := CatalogEntry{
				ID:              itemID,
				StoredPath:      fmt.Sprintf("/test/items/%s.json", itemID),
				MetadataPath:    fmt.Sprintf("/test/metadata/%s.meta", itemID),
				ImportTimestamp: time.Now().Add(time.Duration(i) * time.Second),
				Format:          "json",
				Origin:          fmt.Sprintf("%s-item-%d", coll.id, i),
				Schema:          fmt.Sprintf("%s.#Item", coll.schema[:len(coll.schema)-11]), // Remove "Collection" suffix
				Confidence:      0.8,
				RecordCount:     1,
				SizeBytes:       100,
				CollectionID:    &coll.id,
				ItemIndex:       &itemIndex,
				CollectionType:  &itemType,
				ItemID:          &itemID,
			}

			require.NoError(t, suite.DB.AddEntry(item))
		}
	}

	t.Run("query all collections", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionType: "collection",
		}, QueryOptions{
			SortBy: "schema",
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find all 3 collections
		assert.Equal(t, 3, len(result.Entries))

		// All should be collections
		for _, entry := range result.Entries {
			assert.Equal(t, "collection", *entry.CollectionType)
			assert.Nil(t, entry.CollectionID)
		}
	})

	t.Run("query collections by format", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionType: "collection",
			Format:         "ndjson",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find 2 NDJSON collections
		assert.Equal(t, 2, len(result.Entries))

		for _, entry := range result.Entries {
			assert.Equal(t, "ndjson", entry.Format)
			assert.Equal(t, "collection", *entry.CollectionType)
		}
	})

	t.Run("query items across collections", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionType: "item",
		}, QueryOptions{
			Limit: 15,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find items from all collections (limited to 15)
		assert.Equal(t, 15, len(result.Entries))
		assert.Equal(t, 23, result.FilteredCount) // Total items: 10 + 5 + 8

		// All should be items
		for _, entry := range result.Entries {
			assert.Equal(t, "item", *entry.CollectionType)
			assert.NotNil(t, entry.CollectionID)
			assert.NotNil(t, entry.ItemIndex)
		}
	})

	t.Run("query specific collection items", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionID: "logs-collection",
		}, QueryOptions{
			SortBy: "timestamp",
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find all 10 items from logs collection
		assert.Equal(t, 10, len(result.Entries))

		for i, entry := range result.Entries {
			assert.Equal(t, "logs-collection", *entry.CollectionID)
			assert.Equal(t, i, *entry.ItemIndex)
		}
	})
}

package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/database"
)

// AssertDatabaseEntry validates a catalog entry against expected values
func AssertDatabaseEntry(t *testing.T, expected, actual *database.CatalogEntry) {
	t.Helper()
	require.NotNil(t, actual, "Catalog entry should not be nil")
	
	assert.Equal(t, expected.ID, actual.ID, "Entry ID should match")
	assert.Equal(t, expected.StoredPath, actual.StoredPath, "Stored path should match")
	assert.Equal(t, expected.MetadataPath, actual.MetadataPath, "Metadata path should match")
	assert.Equal(t, expected.Format, actual.Format, "Format should match")
	assert.Equal(t, expected.Origin, actual.Origin, "Origin should match")
	assert.Equal(t, expected.Schema, actual.Schema, "Schema should match")
	assert.Equal(t, expected.RecordCount, actual.RecordCount, "Record count should match")
	assert.Equal(t, expected.SizeBytes, actual.SizeBytes, "Size bytes should match")
	
	// Allow small timestamp differences (up to 1 second)
	timeDiff := actual.ImportTimestamp.Sub(expected.ImportTimestamp)
	assert.True(t, timeDiff >= -time.Second && timeDiff <= time.Second,
		"Import timestamp should be within 1 second of expected")
	
	// Check confidence with small tolerance
	assert.InDelta(t, expected.Confidence, actual.Confidence, 0.01,
		"Confidence should be within 0.01 of expected")
	
	// Check collection-related fields
	if expected.CollectionID == nil {
		assert.Nil(t, actual.CollectionID, "Collection ID should be nil")
	} else {
		require.NotNil(t, actual.CollectionID, "Collection ID should not be nil")
		assert.Equal(t, *expected.CollectionID, *actual.CollectionID, "Collection ID should match")
	}
	
	if expected.ItemIndex == nil {
		assert.Nil(t, actual.ItemIndex, "Item index should be nil")
	} else {
		require.NotNil(t, actual.ItemIndex, "Item index should not be nil")
		assert.Equal(t, *expected.ItemIndex, *actual.ItemIndex, "Item index should match")
	}
	
	if expected.CollectionType == nil {
		assert.Nil(t, actual.CollectionType, "Collection type should be nil")
	} else {
		require.NotNil(t, actual.CollectionType, "Collection type should not be nil")
		assert.Equal(t, *expected.CollectionType, *actual.CollectionType, "Collection type should match")
	}
	
	if expected.ItemID == nil {
		assert.Nil(t, actual.ItemID, "Item ID should be nil")
	} else {
		require.NotNil(t, actual.ItemID, "Item ID should not be nil")
		assert.Equal(t, *expected.ItemID, *actual.ItemID, "Item ID should match")
	}
}

// AssertQueryResult validates a query result structure
func AssertQueryResult(t *testing.T, result *database.QueryResult, expectedCount int) {
	t.Helper()
	require.NotNil(t, result, "Query result should not be nil")
	
	assert.Equal(t, expectedCount, len(result.Entries), "Query result count should match expected")
	assert.GreaterOrEqual(t, result.TotalCount, len(result.Entries), "Total count should be >= result count")
	assert.GreaterOrEqual(t, result.FilteredCount, len(result.Entries), "Filtered count should be >= result count")
}

// AssertQueryResultContains checks if query results contain specific entries
func AssertQueryResultContains(t *testing.T, result *database.QueryResult, expectedIDs []string) {
	t.Helper()
	require.NotNil(t, result, "Query result should not be nil")
	
	actualIDs := make(map[string]bool)
	for _, entry := range result.Entries {
		actualIDs[entry.ID] = true
	}
	
	for _, expectedID := range expectedIDs {
		assert.True(t, actualIDs[expectedID], "Query result should contain entry %s", expectedID)
	}
}

// AssertQueryResultExcludes checks if query results exclude specific entries
func AssertQueryResultExcludes(t *testing.T, result *database.QueryResult, excludedIDs []string) {
	t.Helper()
	require.NotNil(t, result, "Query result should not be nil")
	
	actualIDs := make(map[string]bool)
	for _, entry := range result.Entries {
		actualIDs[entry.ID] = true
	}
	
	for _, excludedID := range excludedIDs {
		assert.False(t, actualIDs[excludedID], "Query result should not contain entry %s", excludedID)
	}
}

// AssertQueryResultOrdered checks if query results are properly ordered
func AssertQueryResultOrdered(t *testing.T, result *database.QueryResult, orderBy string, ascending bool) {
	t.Helper()
	require.NotNil(t, result, "Query result should not be nil")
	
	if len(result.Entries) < 2 {
		return // Nothing to check for ordering
	}
	
	for i := 1; i < len(result.Entries); i++ {
		prev := result.Entries[i-1]
		curr := result.Entries[i]
		
		switch orderBy {
		case "import_timestamp":
			if ascending {
				assert.True(t, !prev.ImportTimestamp.After(curr.ImportTimestamp),
					"Results should be ordered by import timestamp ascending")
			} else {
				assert.True(t, !prev.ImportTimestamp.Before(curr.ImportTimestamp),
					"Results should be ordered by import timestamp descending")
			}
		case "id":
			if ascending {
				assert.True(t, prev.ID <= curr.ID,
					"Results should be ordered by ID ascending")
			} else {
				assert.True(t, prev.ID >= curr.ID,
					"Results should be ordered by ID descending")
			}
		case "size":
			if ascending {
				assert.True(t, prev.SizeBytes <= curr.SizeBytes,
					"Results should be ordered by size ascending")
			} else {
				assert.True(t, prev.SizeBytes >= curr.SizeBytes,
					"Results should be ordered by size descending")
			}
		}
	}
}

// AssertCollectionRelationships validates parent-child relationships in collections
func AssertCollectionRelationships(t *testing.T, collection database.CatalogEntry, items []database.CatalogEntry) {
	t.Helper()
	
	// Validate collection entry
	assert.NotNil(t, collection.CollectionType, "Collection should have collection type")
	if collection.CollectionType != nil {
		assert.Equal(t, "collection", *collection.CollectionType, "Collection type should be 'collection'")
	}
	assert.Nil(t, collection.CollectionID, "Collection should not have parent collection ID")
	assert.Nil(t, collection.ItemIndex, "Collection should not have item index")
	assert.Nil(t, collection.ItemID, "Collection should not have item ID")
	
	// Validate item entries
	for i, item := range items {
		assert.NotNil(t, item.CollectionID, "Item should have collection ID")
		if item.CollectionID != nil {
			assert.Equal(t, collection.ID, *item.CollectionID, "Item collection ID should match collection")
		}
		
		assert.NotNil(t, item.ItemIndex, "Item should have item index")
		if item.ItemIndex != nil {
			assert.Equal(t, i, *item.ItemIndex, "Item index should match position")
		}
		
		assert.NotNil(t, item.CollectionType, "Item should have collection type")
		if item.CollectionType != nil {
			assert.Equal(t, "item", *item.CollectionType, "Item collection type should be 'item'")
		}
		
		assert.NotNil(t, item.ItemID, "Item should have item ID")
		if item.ItemID != nil {
			assert.Equal(t, item.ID, *item.ItemID, "Item ID should match entry ID")
		}
	}
}

// AssertDatabaseEmpty checks if database is empty
func AssertDatabaseEmpty(t *testing.T, db *database.CatalogDB) {
	t.Helper()
	
	result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err, "Query should succeed")
	
	assert.Equal(t, 0, len(result.Entries), "Database should be empty")
	assert.Equal(t, 0, result.TotalCount, "Total count should be zero")
	assert.Equal(t, 0, result.FilteredCount, "Filtered count should be zero")
}

// AssertDatabaseCount checks if database has expected number of entries
func AssertDatabaseCount(t *testing.T, db *database.CatalogDB, expectedCount int) {
	t.Helper()
	
	result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err, "Query should succeed")
	
	assert.Equal(t, expectedCount, len(result.Entries), "Database should have %d entries", expectedCount)
	assert.Equal(t, expectedCount, result.TotalCount, "Total count should be %d", expectedCount)
	assert.Equal(t, expectedCount, result.FilteredCount, "Filtered count should be %d", expectedCount)
}

// AssertEntryExists checks if an entry exists in the database
func AssertEntryExists(t *testing.T, db *database.CatalogDB, entryID string) {
	t.Helper()
	
	entry, err := db.GetEntry(entryID)
	require.NoError(t, err, "GetEntry should succeed for existing entry")
	require.NotNil(t, entry, "Entry should exist")
	assert.Equal(t, entryID, entry.ID, "Retrieved entry should have correct ID")
}

// AssertEntryNotExists checks if an entry does not exist in the database
func AssertEntryNotExists(t *testing.T, db *database.CatalogDB, entryID string) {
	t.Helper()
	
	entry, err := db.GetEntry(entryID)
	assert.Error(t, err, "GetEntry should fail for non-existent entry")
	assert.Nil(t, entry, "Entry should not exist")
}

// AssertFilterResults validates that filter results match expected criteria
func AssertFilterResults(t *testing.T, result *database.QueryResult, filter database.FilterOptions) {
	t.Helper()
	require.NotNil(t, result, "Query result should not be nil")
	
	for _, entry := range result.Entries {
		// Check schema filter
		if filter.Schema != "" {
			assert.Equal(t, filter.Schema, entry.Schema, "Entry schema should match filter")
		}
		
		// Check origin filter
		if filter.Origin != "" {
			assert.Equal(t, filter.Origin, entry.Origin, "Entry origin should match filter")
		}
		
		// Check format filter
		if filter.Format != "" {
			assert.Equal(t, filter.Format, entry.Format, "Entry format should match filter")
		}
		
		// Check collection filters
		if filter.CollectionID != "" {
			if entry.CollectionID != nil {
				assert.Equal(t, filter.CollectionID, *entry.CollectionID, "Entry collection ID should match filter")
			} else {
				t.Errorf("Entry has no collection ID but filter expects: %s", filter.CollectionID)
			}
		}

		if filter.CollectionType != "" {
			if entry.CollectionType != nil {
				assert.Equal(t, filter.CollectionType, *entry.CollectionType, "Entry collection type should match filter")
			} else {
				t.Errorf("Entry has no collection type but filter expects: %s", filter.CollectionType)
			}
		}

		if filter.ItemID != "" {
			if entry.ItemID != nil {
				assert.Equal(t, filter.ItemID, *entry.ItemID, "Entry item ID should match filter")
			} else {
				t.Errorf("Entry has no item ID but filter expects: %s", filter.ItemID)
			}
		}
	}
}

// AssertPaginationResults validates pagination behavior
func AssertPaginationResults(t *testing.T, result *database.QueryResult, options database.QueryOptions, totalExpected int) {
	t.Helper()
	require.NotNil(t, result, "Query result should not be nil")
	
	// Check total count
	assert.Equal(t, totalExpected, result.TotalCount, "Total count should match expected")
	
	// Check result size respects limit
	if options.Limit > 0 {
		expectedSize := options.Limit
		if options.Offset+options.Limit > totalExpected {
			expectedSize = totalExpected - options.Offset
		}
		if expectedSize < 0 {
			expectedSize = 0
		}
		assert.Equal(t, expectedSize, len(result.Entries), "Result size should respect limit and offset")
	}
	
	// Check offset behavior
	if options.Offset > 0 && len(result.Entries) > 0 {
		// We can't easily verify offset without knowing the full dataset order,
		// but we can check that we got some results if offset is within bounds
		assert.True(t, options.Offset < totalExpected, "Offset should be within total count")
	}
}

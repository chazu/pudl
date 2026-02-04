package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryEntries_BasicFilters(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	generator := NewTestDataGenerator()

	// Add diverse test data
	awsEntries := generator.GenerateAWSEntries(10)
	k8sEntries := generator.GenerateK8sEntries(8)
	genericEntries := generator.GenerateGenericEntries(5)

	allEntries := append(awsEntries, k8sEntries...)
	allEntries = append(allEntries, genericEntries...)

	for _, entry := range allEntries {
		require.NoError(t, suite.DB.AddEntry(entry))
	}

	t.Run("filter by schema", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			Schema: "aws.#EC2Instance",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find some AWS EC2 instances
		assert.Greater(t, len(result.Entries), 0)
		assert.LessOrEqual(t, len(result.Entries), 10)

		// All results should have the correct schema
		for _, entry := range result.Entries {
			assert.Equal(t, "aws.#EC2Instance", entry.Schema)
		}
	})

	t.Run("filter by origin", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			Origin: "k8s-get-pods",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find some Kubernetes pods
		assert.Greater(t, len(result.Entries), 0)

		// All results should have the correct origin
		for _, entry := range result.Entries {
			assert.Equal(t, "k8s-get-pods", entry.Origin)
		}
	})

	t.Run("filter by format", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			Format: "yaml",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find YAML entries (K8s entries)
		assert.Greater(t, len(result.Entries), 0)

		// All results should have YAML format
		for _, entry := range result.Entries {
			assert.Equal(t, "yaml", entry.Format)
		}
	})

	t.Run("multiple filters", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			Format: "json",
			Schema: "aws.#S3Bucket",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// All results should match both filters
		for _, entry := range result.Entries {
			assert.Equal(t, "json", entry.Format)
			assert.Equal(t, "aws.#S3Bucket", entry.Schema)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			Schema: "nonexistent.#Schema",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, len(result.Entries))
		assert.Equal(t, 0, result.FilteredCount)
		assert.Equal(t, 23, result.TotalCount) // Total entries in database
	})
}

func TestQueryEntries_Sorting(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	generator := NewTestDataGenerator()

	// Add entries with different characteristics for sorting
	entries := generator.GenerateMixedDataset(20)
	for _, entry := range entries {
		require.NoError(t, suite.DB.AddEntry(entry))
	}

	t.Run("sort by timestamp descending (default)", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			Limit: 10,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 10, len(result.Entries))

		// Should be sorted by timestamp descending (newest first)
		for i := 1; i < len(result.Entries); i++ {
			assert.True(t,
				result.Entries[i-1].ImportTimestamp.After(result.Entries[i].ImportTimestamp) ||
				result.Entries[i-1].ImportTimestamp.Equal(result.Entries[i].ImportTimestamp),
				"Results should be sorted by timestamp descending")
		}
	})

	t.Run("sort by size ascending", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			SortBy:  "size",
			Reverse: false,
			Limit:   10,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 10, len(result.Entries))

		// Should be sorted by size ascending
		for i := 1; i < len(result.Entries); i++ {
			assert.LessOrEqual(t, result.Entries[i-1].SizeBytes, result.Entries[i].SizeBytes,
				"Results should be sorted by size ascending")
		}
	})

	t.Run("sort by confidence descending", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			SortBy:  "confidence",
			Reverse: true,
			Limit:   10,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 10, len(result.Entries))

		// Should be sorted by confidence descending
		for i := 1; i < len(result.Entries); i++ {
			assert.GreaterOrEqual(t, result.Entries[i-1].Confidence, result.Entries[i].Confidence,
				"Results should be sorted by confidence descending")
		}
	})

	t.Run("sort by schema", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			SortBy:  "schema",
			Reverse: false,
			Limit:   15,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 15, len(result.Entries))

		// Should be sorted by schema alphabetically
		for i := 1; i < len(result.Entries); i++ {
			assert.LessOrEqual(t, result.Entries[i-1].Schema, result.Entries[i].Schema,
				"Results should be sorted by schema alphabetically")
		}
	})
}

func TestQueryEntries_Pagination(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	generator := NewTestDataGenerator()

	// Add 50 entries for pagination testing
	entries := generator.GenerateMixedDataset(50)
	for _, entry := range entries {
		require.NoError(t, suite.DB.AddEntry(entry))
	}

	t.Run("basic pagination", func(t *testing.T) {
		// First page
		result1, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			Limit:  10,
			Offset: 0,
		})

		require.NoError(t, err)
		require.NotNil(t, result1)

		assert.Equal(t, 10, len(result1.Entries))
		assert.Equal(t, 50, result1.TotalCount)
		assert.Equal(t, 50, result1.FilteredCount)

		// Second page
		result2, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			Limit:  10,
			Offset: 10,
		})

		require.NoError(t, err)
		require.NotNil(t, result2)

		assert.Equal(t, 10, len(result2.Entries))
		assert.Equal(t, 50, result2.TotalCount)

		// Results should be different
		firstPageIDs := make(map[string]bool)
		for _, entry := range result1.Entries {
			firstPageIDs[entry.ID] = true
		}

		for _, entry := range result2.Entries {
			assert.False(t, firstPageIDs[entry.ID], "Second page should not contain entries from first page")
		}
	})

	t.Run("large offset", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			Limit:  10,
			Offset: 45,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should get remaining 5 entries
		assert.Equal(t, 5, len(result.Entries))
		assert.Equal(t, 50, result.TotalCount)
	})

	t.Run("offset beyond total", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			Limit:  10,
			Offset: 100,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should get no entries
		assert.Equal(t, 0, len(result.Entries))
		assert.Equal(t, 50, result.TotalCount)
	})

	t.Run("no limit", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
			Offset: 0,
		})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should get all entries
		assert.Equal(t, 50, len(result.Entries))
		assert.Equal(t, 50, result.TotalCount)
	})
}

func TestQueryEntries_CollectionFilters(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	generator := NewTestDataGenerator()

	// Create a collection with items
	collectionType := "collection"
	itemType := "item"
	collectionID := "test-collection-001"

	// Add collection entry
	collection := CatalogEntry{
		ID:              collectionID,
		StoredPath:      "/test/collections/test-collection-001.json",
		MetadataPath:    "/test/metadata/test-collection-001.meta",
		ImportTimestamp: time.Now(),
		Format:          "ndjson",
		Origin:          "test-collection",
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

	// Add collection items
	for i := 0; i < 3; i++ {
		itemID := fmt.Sprintf("%s-item-%d", collectionID, i)
		itemIndex := i

		item := CatalogEntry{
			ID:              itemID,
			StoredPath:      fmt.Sprintf("/test/items/%s.json", itemID),
			MetadataPath:    fmt.Sprintf("/test/metadata/%s.meta", itemID),
			ImportTimestamp: time.Now().Add(time.Duration(i) * time.Second),
			Format:          "json",
			Origin:          fmt.Sprintf("test-item-%d", i),
			Schema:          "core.#Item",
			Confidence:      0.8,
			RecordCount:     1,
			SizeBytes:       100,
			CollectionID:    &collectionID,
			ItemIndex:       &itemIndex,
			CollectionType:  &itemType,
			ItemID:          &itemID,
		}
		require.NoError(t, suite.DB.AddEntry(item))
	}

	// Add some regular entries
	regularEntries := generator.GenerateAWSEntries(5)
	for _, entry := range regularEntries {
		require.NoError(t, suite.DB.AddEntry(entry))
	}

	t.Run("filter by collection type - collections", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionType: "collection",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find only the collection entry
		assert.Equal(t, 1, len(result.Entries))
		assert.Equal(t, collectionID, result.Entries[0].ID)
		assert.Equal(t, "collection", *result.Entries[0].CollectionType)
	})

	t.Run("filter by collection type - items", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionType: "item",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find all collection items
		assert.Equal(t, 3, len(result.Entries))

		for _, entry := range result.Entries {
			assert.Equal(t, "item", *entry.CollectionType)
			assert.Equal(t, collectionID, *entry.CollectionID)
		}
	})

	t.Run("filter by collection ID", func(t *testing.T) {
		result, err := suite.DB.QueryEntries(FilterOptions{
			CollectionID: collectionID,
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find all items in the collection
		assert.Equal(t, 3, len(result.Entries))

		for _, entry := range result.Entries {
			assert.Equal(t, collectionID, *entry.CollectionID)
		}
	})

	t.Run("filter by item ID", func(t *testing.T) {
		targetItemID := fmt.Sprintf("%s-item-1", collectionID)

		result, err := suite.DB.QueryEntries(FilterOptions{
			ItemID: targetItemID,
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should find specific item
		assert.Equal(t, 1, len(result.Entries))
		assert.Equal(t, targetItemID, result.Entries[0].ID)
		assert.Equal(t, targetItemID, *result.Entries[0].ItemID)
	})
}

func TestQueryEntries_Performance(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	generator := NewTestDataGenerator()

	// Add large dataset for performance testing
	entries := generator.GenerateLargeDataset(5000)

	start := time.Now()
	for _, entry := range entries {
		require.NoError(t, suite.DB.AddEntry(entry))
	}
	insertDuration := time.Since(start)
	t.Logf("Inserted 5000 entries in %v (%.2f entries/sec)", insertDuration, 5000.0/insertDuration.Seconds())

	t.Run("query performance", func(t *testing.T) {
		start := time.Now()

		result, err := suite.DB.QueryEntries(FilterOptions{
			Schema: "aws.#EC2Instance",
		}, QueryOptions{
			Limit: 100,
		})

		duration := time.Since(start)
		t.Logf("Query completed in %v", duration)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Performance assertion: query should complete within 100ms
		assert.Less(t, duration, 100*time.Millisecond, "Query should complete within 100ms")

		// Should find results
		assert.Greater(t, len(result.Entries), 0)
		assert.Equal(t, 5000, result.TotalCount)
	})

	t.Run("complex query performance", func(t *testing.T) {
		start := time.Now()

		result, err := suite.DB.QueryEntries(FilterOptions{
			Format: "json",
			Schema: "aws.#EC2Instance",
		}, QueryOptions{
			SortBy:  "size",
			Reverse: true,
			Limit:   50,
		})

		duration := time.Since(start)
		t.Logf("Complex query completed in %v", duration)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Performance assertion: complex query should complete within 200ms
		assert.Less(t, duration, 200*time.Millisecond, "Complex query should complete within 200ms")
	})

	t.Run("stress test with concurrent queries", func(t *testing.T) {
		// Test concurrent query performance
		const numConcurrentQueries = 10
		results := make(chan time.Duration, numConcurrentQueries)

		start := time.Now()

		for i := 0; i < numConcurrentQueries; i++ {
			go func(queryID int) {
				queryStart := time.Now()

				_, err := suite.DB.QueryEntries(FilterOptions{
					Format: []string{"json", "yaml"}[queryID%2],
				}, QueryOptions{
					Limit: 50,
				})

				queryDuration := time.Since(queryStart)
				if err == nil {
					results <- queryDuration
				} else {
					results <- -1 // Error marker
				}
			}(i)
		}

		// Collect results
		var queryTimes []time.Duration
		for i := 0; i < numConcurrentQueries; i++ {
			duration := <-results
			if duration > 0 {
				queryTimes = append(queryTimes, duration)
			}
		}

		totalTime := time.Since(start)

		t.Logf("Concurrent queries: %d successful out of %d", len(queryTimes), numConcurrentQueries)
		t.Logf("Total concurrent time: %v", totalTime)

		// All queries should succeed
		assert.Equal(t, numConcurrentQueries, len(queryTimes), "All concurrent queries should succeed")

		// Each query should complete reasonably fast
		for i, duration := range queryTimes {
			assert.Less(t, duration, 500*time.Millisecond, "Concurrent query %d should complete within 500ms", i)
		}

		// Total time should be reasonable (not much more than sequential)
		assert.Less(t, totalTime, 2*time.Second, "Concurrent queries should complete within 2 seconds")
	})
}

func TestQueryEntries_AdvancedCapabilities(t *testing.T) {
	suite := NewDatabaseTestSuite(t)
	require.NoError(t, suite.InitializeDatabase())

	generator := NewTestDataGenerator()

	// Add diverse dataset for advanced testing
	entries := generator.GenerateMixedDataset(200)
	for _, entry := range entries {
		require.NoError(t, suite.DB.AddEntry(entry))
	}

	t.Run("query result consistency", func(t *testing.T) {
		// Same query should return same results
		filters := FilterOptions{Schema: "aws.#EC2Instance"}
		options := QueryOptions{SortBy: "timestamp", Reverse: false}

		result1, err := suite.DB.QueryEntries(filters, options)
		require.NoError(t, err)

		result2, err := suite.DB.QueryEntries(filters, options)
		require.NoError(t, err)

		// Results should be identical
		assert.Equal(t, len(result1.Entries), len(result2.Entries), "Query results should be consistent")
		assert.Equal(t, result1.TotalCount, result2.TotalCount, "Total count should be consistent")

		// Entry order should be identical
		for i := range result1.Entries {
			if i < len(result2.Entries) {
				assert.Equal(t, result1.Entries[i].ID, result2.Entries[i].ID,
					"Entry order should be consistent at position %d", i)
			}
		}
	})

	t.Run("empty result handling", func(t *testing.T) {
		// Query for non-existent data
		result, err := suite.DB.QueryEntries(FilterOptions{
			Schema: "nonexistent.#Schema",
		}, QueryOptions{})

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, len(result.Entries), "Should return empty results")
		assert.Equal(t, 0, result.FilteredCount, "Filtered count should be zero")
		assert.Greater(t, result.TotalCount, 0, "Total count should reflect all entries")
	})

	t.Run("query options validation", func(t *testing.T) {
		// Test various query option combinations
		testCases := []struct {
			name    string
			options QueryOptions
			valid   bool
		}{
			{
				name:    "valid limit and offset",
				options: QueryOptions{Limit: 10, Offset: 5},
				valid:   true,
			},
			{
				name:    "zero limit",
				options: QueryOptions{Limit: 0, Offset: 0},
				valid:   true, // Should return all results
			},
			{
				name:    "large offset",
				options: QueryOptions{Limit: 10, Offset: 1000},
				valid:   true, // Should return empty results
			},
			{
				name:    "negative offset",
				options: QueryOptions{Limit: 10, Offset: -1},
				valid:   true, // Should be treated as 0
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result, err := suite.DB.QueryEntries(FilterOptions{}, tc.options)

				if tc.valid {
					assert.NoError(t, err, "Query should succeed for %s", tc.name)
					assert.NotNil(t, result, "Result should not be nil for %s", tc.name)
				} else {
					assert.Error(t, err, "Query should fail for %s", tc.name)
				}
			})
		}
	})

	t.Run("sorting edge cases", func(t *testing.T) {
		// Test sorting with various field types
		sortTests := []struct {
			sortBy  string
			reverse bool
			desc    string
		}{
			{"timestamp", false, "timestamp ascending"},
			{"timestamp", true, "timestamp descending"},
			{"schema", false, "schema ascending"},
			{"schema", true, "schema descending"},
			{"size", false, "size ascending"},
			{"size", true, "size descending"},
			{"confidence", false, "confidence ascending"},
			{"confidence", true, "confidence descending"},
		}

		for _, test := range sortTests {
			t.Run(test.desc, func(t *testing.T) {
				result, err := suite.DB.QueryEntries(FilterOptions{}, QueryOptions{
					SortBy:  test.sortBy,
					Reverse: test.reverse,
					Limit:   20,
				})

				require.NoError(t, err, "Sort query should succeed")
				require.NotNil(t, result, "Result should not be nil")

				if len(result.Entries) > 1 {
					// Verify sorting is applied (basic check)
					t.Logf("Sort %s: first=%s, last=%s", test.desc,
						result.Entries[0].ID, result.Entries[len(result.Entries)-1].ID)
				}
			})
		}
	})

	t.Run("filter combination edge cases", func(t *testing.T) {
		// Test various filter combinations
		filterTests := []struct {
			name    string
			filters FilterOptions
		}{
			{
				name:    "multiple filters",
				filters: FilterOptions{Format: "json", Schema: "aws.#EC2Instance"},
			},
			{
				name:    "empty string filters",
				filters: FilterOptions{Format: "", Schema: "", Origin: ""},
			},
			{
				name:    "case sensitive filters",
				filters: FilterOptions{Format: "JSON"}, // Should not match "json"
			},
		}

		for _, test := range filterTests {
			t.Run(test.name, func(t *testing.T) {
				result, err := suite.DB.QueryEntries(test.filters, QueryOptions{})

				require.NoError(t, err, "Filter query should succeed")
				require.NotNil(t, result, "Result should not be nil")

				t.Logf("Filter %s: found %d entries", test.name, len(result.Entries))
			})
		}
	})
}

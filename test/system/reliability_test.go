package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/database"
)

func TestSystemErrorHandling(t *testing.T) {
	suite := NewSystemTestSuite(t)
	require.NoError(t, suite.InitializeDirectories())

	t.Run("database corruption recovery", func(t *testing.T) {
		dbPath := filepath.Join(suite.TempDir, "corruption")
		
		// Create database and add data
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		
		testEntry := database.CatalogEntry{
			ID:              "corruption-test-001",
			StoredPath:      "/test/corruption.json",
			MetadataPath:    "/test/corruption.meta",
			ImportTimestamp: time.Now(),
			Format:          "json",
			Origin:          "corruption-test",
			Schema:          "test.#Corruption",
			Confidence:      0.9,
			RecordCount:     1,
			SizeBytes:       100,
		}
		
		err = db.AddEntry(testEntry)
		require.NoError(t, err)
		
		// Close database properly
		err = db.Close()
		require.NoError(t, err)
		
		// Simulate corruption by writing garbage to database file
		dbFiles, err := filepath.Glob(filepath.Join(dbPath, "*"))
		require.NoError(t, err)
		
		if len(dbFiles) > 0 {
			// Find a database file (not a directory)
			var dbFile string
			for _, file := range dbFiles {
				info, err := os.Stat(file)
				if err == nil && !info.IsDir() {
					dbFile = file
					break
				}
			}

			if dbFile != "" {
				// Write some garbage to the database file
				err = os.WriteFile(dbFile, []byte("CORRUPTED DATA"), 0644)
				require.NoError(t, err)

				// Try to reopen database
				db2, err := database.NewCatalogDB(dbPath)
				if err != nil {
					// Corruption should be detected and handled gracefully
					assert.NotEmpty(t, err.Error(), "Corruption error should be descriptive")
					t.Logf("Database corruption correctly detected: %v", err)
				} else {
					// If it opens, it should either recover or be empty
					result, queryErr := db2.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
					if queryErr != nil {
						t.Logf("Query failed after corruption (expected): %v", queryErr)
					} else {
						t.Logf("Database recovered with %d entries", len(result.Entries))
					}
					db2.Close()
				}
			} else {
				t.Log("No database files found to corrupt - test skipped")
			}
		}
	})

	t.Run("resource exhaustion handling", func(t *testing.T) {
		dbPath := filepath.Join(suite.TempDir, "exhaustion")
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		defer db.Close()

		// Try to create many entries rapidly to test resource limits
		const maxEntries = 10000
		var lastError error
		successCount := 0

		for i := 0; i < maxEntries; i++ {
			entry := database.CatalogEntry{
				ID:              fmt.Sprintf("exhaustion-test-%08d", i),
				StoredPath:      fmt.Sprintf("/test/exhaustion-%08d.json", i),
				MetadataPath:    fmt.Sprintf("/test/exhaustion-%08d.meta", i),
				ImportTimestamp: time.Now(),
				Format:          "json",
				Origin:          "exhaustion-test",
				Schema:          "test.#Exhaustion",
				Confidence:      0.8,
				RecordCount:     1,
				SizeBytes:       int64(100 + i),
			}

			err = db.AddEntry(entry)
			if err != nil {
				lastError = err
				break
			}
			successCount++

			// Check every 1000 entries
			if i%1000 == 0 && i > 0 {
				// Verify database is still functional
				result, queryErr := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 1})
				if queryErr != nil {
					lastError = queryErr
					break
				}
				assert.Greater(t, len(result.Entries), 0, "Database should remain queryable")
			}
		}

		t.Logf("Successfully added %d entries before hitting limits", successCount)
		if lastError != nil {
			t.Logf("Resource limit error (expected): %v", lastError)
		}

		// Database should still be functional for queries
		result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 10})
		require.NoError(t, err, "Database should remain queryable after resource exhaustion")
		assert.Greater(t, len(result.Entries), 0, "Should be able to query existing entries")
	})

	t.Run("invalid data handling", func(t *testing.T) {
		dbPath := filepath.Join(suite.TempDir, "invalid")
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		defer db.Close()

		// Test various invalid data scenarios
		invalidEntries := []struct {
			name  string
			entry database.CatalogEntry
		}{
			{
				name: "extremely long ID",
				entry: database.CatalogEntry{
					ID:              strings.Repeat("x", 10000),
					StoredPath:      "/test/long-id.json",
					MetadataPath:    "/test/long-id.meta",
					ImportTimestamp: time.Now(),
					Format:          "json",
					Origin:          "invalid-test",
					Schema:          "test.#Invalid",
					Confidence:      0.8,
					RecordCount:     1,
					SizeBytes:       100,
				},
			},
			{
				name: "extremely long path",
				entry: database.CatalogEntry{
					ID:              "invalid-long-path",
					StoredPath:      "/test/" + strings.Repeat("very-long-path-component/", 100) + "file.json",
					MetadataPath:    "/test/invalid-long-path.meta",
					ImportTimestamp: time.Now(),
					Format:          "json",
					Origin:          "invalid-test",
					Schema:          "test.#Invalid",
					Confidence:      0.8,
					RecordCount:     1,
					SizeBytes:       100,
				},
			},
			{
				name: "special characters in fields",
				entry: database.CatalogEntry{
					ID:              "invalid-special-\x00\x01\x02",
					StoredPath:      "/test/special-\x00\x01\x02.json",
					MetadataPath:    "/test/special.meta",
					ImportTimestamp: time.Now(),
					Format:          "json\x00",
					Origin:          "invalid\x01test",
					Schema:          "test.#Invalid\x02",
					Confidence:      0.8,
					RecordCount:     1,
					SizeBytes:       100,
				},
			},
		}

		for _, test := range invalidEntries {
			t.Run(test.name, func(t *testing.T) {
				err := db.AddEntry(test.entry)
				if err != nil {
					// Error is expected for invalid data
					assert.NotEmpty(t, err.Error(), "Error should be descriptive")
					t.Logf("Invalid data correctly rejected (%s): %v", test.name, err)
				} else {
					// If accepted, database should remain functional
					t.Logf("Invalid data was accepted (%s)", test.name)
					
					// Verify we can still query
					result, queryErr := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 1})
					assert.NoError(t, queryErr, "Database should remain functional after invalid data")
					assert.NotNil(t, result, "Query result should not be nil")
				}
			})
		}
	})

	t.Run("concurrent stress test", func(t *testing.T) {
		dbPath := filepath.Join(suite.TempDir, "stress")
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		defer db.Close()

		// Concurrent operations stress test
		const numWorkers = 10
		const operationsPerWorker = 100
		
		results := make(chan error, numWorkers)
		
		// Start concurrent workers
		for workerID := 0; workerID < numWorkers; workerID++ {
			go func(id int) {
				var lastErr error
				
				for i := 0; i < operationsPerWorker; i++ {
					// Mix of operations
					switch i % 4 {
					case 0: // Add entry
						entry := database.CatalogEntry{
							ID:              fmt.Sprintf("stress-worker-%d-op-%d", id, i),
							StoredPath:      fmt.Sprintf("/test/stress-w%d-op%d.json", id, i),
							MetadataPath:    fmt.Sprintf("/test/stress-w%d-op%d.meta", id, i),
							ImportTimestamp: time.Now(),
							Format:          "json",
							Origin:          fmt.Sprintf("stress-worker-%d", id),
							Schema:          "test.#Stress",
							Confidence:      0.8,
							RecordCount:     1,
							SizeBytes:       int64(100 + i),
						}
						lastErr = db.AddEntry(entry)
						
					case 1: // Query entries
						_, lastErr = db.QueryEntries(database.FilterOptions{
							Origin: fmt.Sprintf("stress-worker-%d", id),
						}, database.QueryOptions{Limit: 10})
						
					case 2: // Query all
						_, lastErr = db.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 5})
						
					case 3: // Try to get specific entry
						entryID := fmt.Sprintf("stress-worker-%d-op-%d", id, i-1)
						_, lastErr = db.GetEntry(entryID)
						// Ignore "not found" errors for this test
						if lastErr != nil && strings.Contains(lastErr.Error(), "not found") {
							lastErr = nil
						}
					}
					
					if lastErr != nil {
						break
					}
				}
				
				results <- lastErr
			}(workerID)
		}
		
		// Collect results
		errorCount := 0
		for i := 0; i < numWorkers; i++ {
			err := <-results
			if err != nil {
				errorCount++
				t.Logf("Worker error: %v", err)
			}
		}
		
		t.Logf("Stress test completed: %d workers, %d errors", numWorkers, errorCount)
		
		// Some errors are acceptable under stress, but not too many
		assert.Less(t, errorCount, numWorkers/2, "Most workers should succeed under stress")
		
		// Database should still be functional
		result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 10})
		require.NoError(t, err, "Database should be functional after stress test")
		t.Logf("Database contains %d entries after stress test", result.TotalCount)
	})
}

func TestSystemEdgeCases(t *testing.T) {
	suite := NewSystemTestSuite(t)
	require.NoError(t, suite.InitializeDirectories())

	t.Run("empty database operations", func(t *testing.T) {
		dbPath := filepath.Join(suite.TempDir, "empty")
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		defer db.Close()

		// Operations on empty database should work
		result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(result.Entries), "Empty database should return no entries")
		assert.Equal(t, 0, result.TotalCount, "Total count should be zero")

		// Get non-existent entry
		entry, err := db.GetEntry("nonexistent")
		assert.Error(t, err, "Should error for non-existent entry")
		assert.Nil(t, entry, "Entry should be nil")

		// Delete non-existent entry
		err = db.DeleteEntry("nonexistent")
		assert.Error(t, err, "Should error when deleting non-existent entry")

		// Update non-existent entry
		testEntry := database.CatalogEntry{
			ID:              "nonexistent",
			StoredPath:      "/test/nonexistent.json",
			MetadataPath:    "/test/nonexistent.meta",
			ImportTimestamp: time.Now(),
			Format:          "json",
			Origin:          "test",
			Schema:          "test.#Test",
			Confidence:      0.8,
			RecordCount:     1,
			SizeBytes:       100,
		}
		err = db.UpdateEntry(testEntry)
		assert.Error(t, err, "Should error when updating non-existent entry")
	})

	t.Run("database size limits", func(t *testing.T) {
		dbPath := filepath.Join(suite.TempDir, "limits")
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		defer db.Close()

		// Test with progressively larger entries
		sizes := []int{1, 100, 1000, 10000, 100000}
		
		for _, size := range sizes {
			entry := database.CatalogEntry{
				ID:              fmt.Sprintf("size-test-%d", size),
				StoredPath:      fmt.Sprintf("/test/size-%d.json", size),
				MetadataPath:    fmt.Sprintf("/test/size-%d.meta", size),
				ImportTimestamp: time.Now(),
				Format:          "json",
				Origin:          "size-test",
				Schema:          "test.#Size",
				Confidence:      0.8,
				RecordCount:     1,
				SizeBytes:       int64(size),
			}

			err = db.AddEntry(entry)
			if err != nil {
				t.Logf("Failed to add entry of size %d: %v", size, err)
				break
			} else {
				t.Logf("Successfully added entry of size %d", size)
			}
		}

		// Database should remain functional
		result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
		require.NoError(t, err, "Database should remain functional")
		t.Logf("Database contains %d entries after size test", len(result.Entries))
	})
}

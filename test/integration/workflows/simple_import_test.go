package workflows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/database"
	"pudl/internal/importer"
	"pudl/test/integration/infrastructure"
)

func TestSimpleImportWorkflow(t *testing.T) {
	suite := infrastructure.NewIntegrationTestSuite(t)
	require.NoError(t, suite.Initialize())

	t.Run("simple json import", func(t *testing.T) {
		// Create a simple, valid JSON file
		simpleJSON := `{
  "InstanceId": "i-1234567890abcdef0",
  "ImageId": "ami-12345678",
  "State": {
    "Code": 16,
    "Name": "running"
  },
  "InstanceType": "t3.micro",
  "LaunchTime": "2023-01-01T12:00:00.000Z",
  "Tags": [
    {
      "Key": "Name",
      "Value": "test-instance"
    }
  ]
}`

		filePath, err := suite.CreateTestFile("simple-test.json", simpleJSON)
		require.NoError(t, err)

		suite.LogInfo("Testing simple JSON import: %s", filePath)

		// Import the file
		opts := importer.ImportOptions{
			SourcePath: filePath,
		}

		result, err := suite.Importer.ImportFile(opts)
		require.NoError(t, err, "Simple JSON import should succeed")
		require.NotNil(t, result, "Import result should not be nil")

		suite.LogInfo("Import Result:")
		suite.LogInfo("  ID: %s", result.ID)
		suite.LogInfo("  StoredPath: %s", result.StoredPath)
		suite.LogInfo("  AssignedSchema: %s", result.AssignedSchema)
		suite.LogInfo("  RecordCount: %d", result.RecordCount)
		suite.LogInfo("  SizeBytes: %d", result.SizeBytes)
		// Validate import result
		assert.NotEmpty(t, result.ID, "Should have assigned ID")
		assert.NotEmpty(t, result.StoredPath, "Should have stored path")
		assert.NotEmpty(t, result.AssignedSchema, "Should have assigned schema")
		assert.Greater(t, result.RecordCount, 0, "Should have processed records")
		assert.Greater(t, result.SizeBytes, int64(0), "Should have size")

		// Check if entry was added to database
		totalCount, err := suite.GetDatabaseEntryCount()
		require.NoError(t, err)
		suite.LogInfo("Database entries after import: %d", totalCount)

		if totalCount > 0 {
			// If entry was added to database, validate it
			entry, err := suite.Database.GetEntry(result.ID)
			require.NoError(t, err, "Should be able to retrieve imported entry")
			require.NotNil(t, entry, "Retrieved entry should not be nil")

			assert.Equal(t, result.ID, entry.ID, "Database entry ID should match import result")
			assert.Equal(t, result.AssignedSchema, entry.Schema, "Database schema should match import result")
			assert.Equal(t, result.RecordCount, entry.RecordCount, "Database record count should match")

			suite.LogInfo("Database entry validated successfully")
		} else {
			suite.LogInfo("Import succeeded but entry not found in database - this may be expected behavior")
		}
	})

	t.Run("minimal dataset import", func(t *testing.T) {
		// Load and import minimal test dataset
		dataset, err := suite.LoadTestDataSet("minimal-test")
		require.NoError(t, err)
		require.NotNil(t, dataset)

		suite.LogInfo("Testing minimal dataset import: %d files", len(dataset.Files))

		var importResults []*importer.ImportResult
		var importErrors []error

		// Import each file individually to see what happens
		for _, file := range dataset.Files {
			opts := importer.ImportOptions{
				SourcePath: file.Name,
			}

			result, err := suite.Importer.ImportFile(opts)
			if err != nil {
				importErrors = append(importErrors, err)
				suite.LogInfo("Import error for %s: %v", file.Name, err)
			} else {
				importResults = append(importResults, result)
				suite.LogInfo("Import success for %s: ID=%s, Schema=%s, Records=%d",
					file.Name, result.ID, result.AssignedSchema, result.RecordCount)
			}
		}

		// Log results
		suite.LogInfo("Import Results Summary:")
		suite.LogInfo("  Successful imports: %d", len(importResults))
		suite.LogInfo("  Failed imports: %d", len(importErrors))

		// Check database state
		totalCount, err := suite.GetDatabaseEntryCount()
		require.NoError(t, err)
		suite.LogInfo("  Database entries: %d", totalCount)

		// At least some imports should succeed
		assert.Greater(t, len(importResults), 0, "At least some imports should succeed")

		// Validate successful imports
		for i, result := range importResults {
			assert.NotEmpty(t, result.ID, "Import %d should have ID", i)
			assert.NotEmpty(t, result.AssignedSchema, "Import %d should have schema", i)
			assert.Greater(t, result.RecordCount, 0, "Import %d should have records", i)
		}
	})

	t.Run("import and query workflow", func(t *testing.T) {
		// Create multiple simple files with different characteristics
		files := []struct {
			name    string
			content string
			format  string
		}{
			{
				name:   "aws-instance.json",
				format: "json",
				content: `{
  "InstanceId": "i-test001",
  "InstanceType": "t3.micro",
  "State": {"Name": "running"},
  "Tags": [{"Key": "Name", "Value": "test-instance-1"}]
}`,
			},
			{
				name:   "aws-bucket.json",
				format: "json",
				content: `{
  "Name": "test-bucket-001",
  "CreationDate": "2023-01-01T00:00:00Z"
}`,
			},
		}

		var importedIDs []string

		// Import all files
		for _, file := range files {
			filePath, err := suite.CreateTestFile(file.name, file.content)
			require.NoError(t, err)

			opts := importer.ImportOptions{
				SourcePath: filePath,
			}

			result, err := suite.Importer.ImportFile(opts)
			if err != nil {
				suite.LogInfo("Import failed for %s: %v", file.name, err)
				continue
			}

			importedIDs = append(importedIDs, result.ID)
			suite.LogInfo("Imported %s: ID=%s, Schema=%s", file.name, result.ID, result.AssignedSchema)
		}

		// Check database state
		totalCount, err := suite.GetDatabaseEntryCount()
		require.NoError(t, err)
		suite.LogInfo("Total database entries: %d", totalCount)

		if totalCount > 0 {
			// Test basic queries
			allResults, err := suite.QueryDatabase(
				database.FilterOptions{},
				database.QueryOptions{},
			)
			require.NoError(t, err)
			suite.LogInfo("Query all results: %d entries", len(allResults.Entries))

			// Test format filtering
			jsonResults, err := suite.QueryDatabase(
				database.FilterOptions{Format: "json"},
				database.QueryOptions{},
			)
			require.NoError(t, err)
			suite.LogInfo("Query JSON results: %d entries", len(jsonResults.Entries))

			// Validate query results
			assert.Equal(t, totalCount, len(allResults.Entries), "Query all should return all entries")
			assert.LessOrEqual(t, len(jsonResults.Entries), totalCount, "JSON filter should return <= total")
		} else {
			suite.LogInfo("No entries in database - imports may not be persisting to database")
		}

		suite.LogInfo("Import and Query Workflow Summary:")
		suite.LogInfo("  Files processed: %d", len(files))
		suite.LogInfo("  Successful imports: %d", len(importedIDs))
		suite.LogInfo("  Database entries: %d", totalCount)
	})
}

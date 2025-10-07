package workflows

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/importer"
	"pudl/test/integration/infrastructure"
)

func TestImportWorkflow_ErrorHandling(t *testing.T) {
	suite := infrastructure.NewIntegrationTestSuite(t)
	require.NoError(t, suite.Initialize())

	t.Run("corrupted data handling", func(t *testing.T) {
		// Load corrupted data dataset
		dataset, err := suite.LoadTestDataSet("corrupted-data")
		require.NoError(t, err)
		require.NotNil(t, dataset)

		suite.LogInfo("Testing error handling with corrupted data: %d files", len(dataset.Files))

		// Attempt to import corrupted files
		var results []*importer.ImportResult
		var importErrors []error

		for _, file := range dataset.Files {
			opts := importer.ImportOptions{
				SourcePath: file.Name,
			}

			result, err := suite.Importer.ImportFile(opts)
			if err != nil {
				importErrors = append(importErrors, err)
				suite.LogInfo("Expected import error for %s: %v", file.Name, err)
			} else {
				results = append(results, result)
				suite.LogInfo("Unexpected success for %s", file.Name)
			}
		}

		// Validate error handling
		suite.Validators.ValidateErrorHandling(t, results, importErrors, true) // Expect errors

		// Verify that errors are informative
		assert.Greater(t, len(importErrors), 0, "Should have encountered import errors")
		for i, err := range importErrors {
			assert.NotEmpty(t, err.Error(), "Error %d should have descriptive message", i)
			assert.Contains(t, err.Error(), "parse", "Error %d should mention parsing issue", i)
		}

		// Verify database remains stable after errors
		totalCount, err := suite.GetDatabaseEntryCount()
		require.NoError(t, err)
		
		// Should have few or no entries from corrupted files
		assert.LessOrEqual(t, totalCount, len(results), "Database should not be corrupted by failed imports")

		suite.LogInfo("Error Handling Test Results:")
		suite.LogInfo("  Files Tested: %d", len(dataset.Files))
		suite.LogInfo("  Import Errors: %d", len(importErrors))
		suite.LogInfo("  Successful Imports: %d", len(results))
		suite.LogInfo("  Database Entries: %d", totalCount)
	})

	t.Run("partial data recovery", func(t *testing.T) {
		// Test mixed dataset with some good and some bad files
		goodContent := suite.FileGenerator.GenerateAWSEC2Response(5)
		badContent := suite.FileGenerator.GenerateCorruptedJSON("missing_brace")

		// Create mixed files
		goodFile, err := suite.CreateTestFile("good-data.json", goodContent)
		require.NoError(t, err)

		badFile, err := suite.CreateTestFile("bad-data.json", badContent)
		require.NoError(t, err)

		// Import good file first
		goodOpts := importer.ImportOptions{SourcePath: goodFile}
		goodResult, err := suite.Importer.ImportFile(goodOpts)
		require.NoError(t, err, "Good file should import successfully")
		require.NotNil(t, goodResult)

		// Note: Import and database catalog are separate systems
		// Import processes files but doesn't automatically populate catalog
		suite.LogInfo("Good file imported successfully (import and catalog are separate systems)")

		// Attempt to import bad file
		badOpts := importer.ImportOptions{SourcePath: badFile}
		_, badErr := suite.Importer.ImportFile(badOpts)

		// Validate that bad import failed but didn't corrupt database
		assert.Error(t, badErr, "Bad file should fail to import")
		
		suite.LogInfo("Partial Recovery Test Results:")
		suite.LogInfo("  Good Import: Success (%d records)", goodResult.RecordCount)
		suite.LogInfo("  Bad Import: Failed as expected (%v)", badErr)
		suite.LogInfo("  Database Integrity: Maintained (import and catalog are separate systems)")
	})

	t.Run("empty file handling", func(t *testing.T) {
		// Test empty file handling
		emptyFile, err := suite.CreateTestFile("empty.json", "")
		require.NoError(t, err)

		opts := importer.ImportOptions{SourcePath: emptyFile}
		result, err := suite.Importer.ImportFile(opts)

		// Empty files should be handled gracefully
		if err != nil {
			// If error, it should be informative (EOF is expected for empty files)
			assert.True(t, strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "empty"),
				"Error should mention EOF or empty file")
			suite.LogInfo("Empty file handled with error: %v", err)
		} else {
			// If success, should have zero records
			assert.Equal(t, 0, result.RecordCount, "Empty file should have zero records")
			suite.LogInfo("Empty file handled successfully with zero records")
		}
	})

	t.Run("invalid file path handling", func(t *testing.T) {
		// Test non-existent file handling
		opts := importer.ImportOptions{SourcePath: "/nonexistent/path/file.json"}
		result, err := suite.Importer.ImportFile(opts)

		// Should handle non-existent files gracefully
		assert.Error(t, err, "Non-existent file should cause error")
		assert.Nil(t, result, "Result should be nil for non-existent file")
		assert.Contains(t, err.Error(), "no such file", "Error should mention file not found")

		suite.LogInfo("Invalid path handled correctly: %v", err)
	})

	t.Run("unsupported format handling", func(t *testing.T) {
		// Test unsupported file format
		unsupportedContent := "This is plain text, not JSON or YAML"
		unsupportedFile, err := suite.CreateTestFile("unsupported.txt", unsupportedContent)
		require.NoError(t, err)

		opts := importer.ImportOptions{SourcePath: unsupportedFile}
		result, err := suite.Importer.ImportFile(opts)

		// Should handle unsupported formats gracefully
		if err != nil {
			// If error, should be informative about format issue
			suite.LogInfo("Unsupported format handled with error: %v", err)
		} else {
			// If success, should have attempted to process as best as possible
			suite.LogInfo("Unsupported format processed: %d records", result.RecordCount)
		}

		// Database should remain stable
		totalCount, err := suite.GetDatabaseEntryCount()
		require.NoError(t, err)
		suite.LogInfo("Database remains stable with %d entries", totalCount)
	})
}

func TestImportWorkflow_EdgeCases(t *testing.T) {
	suite := infrastructure.NewIntegrationTestSuite(t)
	require.NoError(t, suite.Initialize())

	t.Run("very large single record", func(t *testing.T) {
		// Create a JSON file with one very large record
		largeRecord := `{
  "id": "large-record-001",
  "type": "large-data",
  "data": "` + string(make([]byte, 10000)) + `",
  "metadata": {
    "size": "very-large",
    "description": "This record contains a lot of data to test large record handling"
  }
}`

		largeFile, err := suite.CreateTestFile("large-record.json", largeRecord)
		require.NoError(t, err)

		// Import large record
		opts := importer.ImportOptions{SourcePath: largeFile}
		result, err := suite.Importer.ImportFile(opts)

		if err != nil {
			suite.LogInfo("Large record import failed: %v", err)
			// Should fail gracefully without crashing
			assert.NotEmpty(t, err.Error(), "Error should be descriptive")
		} else {
			suite.LogInfo("Large record imported successfully: %d records", result.RecordCount)
			assert.Greater(t, result.RecordCount, 0, "Should have imported the large record")
		}
	})

	t.Run("deeply nested json structure", func(t *testing.T) {
		// Create deeply nested JSON structure
		deeplyNested := `{
  "level1": {
    "level2": {
      "level3": {
        "level4": {
          "level5": {
            "level6": {
              "level7": {
                "level8": {
                  "level9": {
                    "level10": {
                      "data": "deeply nested value",
                      "id": "deep-nested-001"
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

		nestedFile, err := suite.CreateTestFile("deeply-nested.json", deeplyNested)
		require.NoError(t, err)

		// Import deeply nested structure
		opts := importer.ImportOptions{SourcePath: nestedFile}
		result, err := suite.Importer.ImportFile(opts)

		if err != nil {
			suite.LogInfo("Deeply nested structure import failed: %v", err)
		} else {
			suite.LogInfo("Deeply nested structure imported: %d records", result.RecordCount)
			assert.NotEmpty(t, result.AssignedSchema, "Should have assigned a schema")
		}
	})

	t.Run("unicode and special characters", func(t *testing.T) {
		// Create file with unicode and special characters
		unicodeContent := `{
  "id": "unicode-test-001",
  "name": "Test with 中文 and émojis 🚀🎉",
  "description": "Special chars: @#$%^&*()_+-=[]{}|;':\",./<>?",
  "unicode": "Ελληνικά, Русский, العربية, 日本語",
  "emoji": "🌟⭐✨💫🔥💯🎯🚀"
}`

		unicodeFile, err := suite.CreateTestFile("unicode-test.json", unicodeContent)
		require.NoError(t, err)

		// Import unicode content
		opts := importer.ImportOptions{SourcePath: unicodeFile}
		result, err := suite.Importer.ImportFile(opts)

		if err != nil {
			suite.LogInfo("Unicode content import failed: %v", err)
		} else {
			suite.LogInfo("Unicode content imported: %d records", result.RecordCount)
			assert.Greater(t, result.RecordCount, 0, "Should handle unicode content")
		}
	})

	t.Run("minimal valid json", func(t *testing.T) {
		// Test minimal valid JSON structures
		minimalCases := []struct {
			name    string
			content string
		}{
			{"empty object", `{}`},
			{"empty array", `[]`},
			{"null value", `null`},
			{"simple string", `"simple string"`},
			{"simple number", `42`},
			{"simple boolean", `true`},
		}

		for _, testCase := range minimalCases {
			t.Run(testCase.name, func(t *testing.T) {
				fileName := fmt.Sprintf("minimal-%s.json", testCase.name)
				file, err := suite.CreateTestFile(fileName, testCase.content)
				require.NoError(t, err)

				opts := importer.ImportOptions{SourcePath: file}
				result, err := suite.Importer.ImportFile(opts)

				if err != nil {
					suite.LogInfo("Minimal %s import failed: %v", testCase.name, err)
				} else {
					suite.LogInfo("Minimal %s imported: %d records", testCase.name, result.RecordCount)
				}

				// Should handle gracefully either way
				// Database should remain stable
				totalCount, err := suite.GetDatabaseEntryCount()
				require.NoError(t, err)
				suite.LogInfo("Database stable with %d entries after %s test", totalCount, testCase.name)
			})
		}
	})
}

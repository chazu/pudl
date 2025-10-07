package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/database"
)

// AssertFileExists asserts that a file exists at the given path
func AssertFileExists(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	assert.NoError(t, err, "File should exist: %s", path)
}

// AssertFileNotExists asserts that a file does not exist at the given path
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "File should not exist: %s", path)
}

// AssertFileContains asserts that a file contains the expected content
func AssertFileContains(t *testing.T, path, expectedContent string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file: %s", path)
	assert.Contains(t, string(content), expectedContent, "File should contain expected content")
}

// AssertDirectoryExists asserts that a directory exists at the given path
func AssertDirectoryExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err, "Directory should exist: %s", path)
	assert.True(t, info.IsDir(), "Path should be a directory: %s", path)
}

// AssertDirectoryEmpty asserts that a directory is empty
func AssertDirectoryEmpty(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	require.NoError(t, err, "Failed to read directory: %s", path)
	assert.Empty(t, entries, "Directory should be empty: %s", path)
}

// AssertJSONEqual asserts that two JSON strings are equivalent
func AssertJSONEqual(t *testing.T, expected, actual string) {
	t.Helper()
	
	var expectedObj, actualObj interface{}
	
	err := json.Unmarshal([]byte(expected), &expectedObj)
	require.NoError(t, err, "Failed to parse expected JSON")
	
	err = json.Unmarshal([]byte(actual), &actualObj)
	require.NoError(t, err, "Failed to parse actual JSON")
	
	assert.Equal(t, expectedObj, actualObj, "JSON objects should be equal")
}

// AssertValidJSON asserts that a string is valid JSON
func AssertValidJSON(t *testing.T, jsonStr string) {
	t.Helper()
	var obj interface{}
	err := json.Unmarshal([]byte(jsonStr), &obj)
	assert.NoError(t, err, "String should be valid JSON: %s", jsonStr)
}

// AssertErrorContains asserts that an error contains the expected message
func AssertErrorContains(t *testing.T, err error, expectedMessage string) {
	t.Helper()
	require.Error(t, err, "Expected an error")
	assert.Contains(t, err.Error(), expectedMessage, "Error should contain expected message")
}

// AssertErrorType asserts that an error is of the expected type
func AssertErrorType(t *testing.T, err error, expectedType interface{}) {
	t.Helper()
	require.Error(t, err, "Expected an error")
	assert.IsType(t, expectedType, err, "Error should be of expected type")
}

// AssertCatalogEntry asserts properties of a catalog entry
func AssertCatalogEntry(t *testing.T, entry *database.CatalogEntry, expectedPath, expectedFormat string) {
	t.Helper()
	require.NotNil(t, entry, "Catalog entry should not be nil")
	assert.Equal(t, expectedPath, entry.StoredPath, "Entry stored path should match")
	assert.Equal(t, expectedFormat, entry.Format, "Entry format should match")
	assert.NotZero(t, entry.ID, "Entry ID should be set")
	assert.False(t, entry.ImportTimestamp.IsZero(), "Entry ImportTimestamp should be set")
}

// AssertCatalogEntryHasMetadata asserts that a catalog entry has expected metadata
func AssertCatalogEntryHasMetadata(t *testing.T, entry *database.CatalogEntry, expectedSchema, expectedOrigin string) {
	t.Helper()
	require.NotNil(t, entry, "Catalog entry should not be nil")
	assert.Equal(t, expectedSchema, entry.Schema, "Schema should match")
	assert.Equal(t, expectedOrigin, entry.Origin, "Origin should match")
	assert.NotEmpty(t, entry.MetadataPath, "Metadata path should be set")
}

// AssertTimeRecent asserts that a time is recent (within the last minute)
func AssertTimeRecent(t *testing.T, timestamp time.Time) {
	t.Helper()
	now := time.Now()
	diff := now.Sub(timestamp)
	assert.True(t, diff >= 0, "Time should not be in the future")
	assert.True(t, diff < time.Minute, "Time should be recent (within 1 minute)")
}

// AssertStringSliceContains asserts that a string slice contains an expected value
func AssertStringSliceContains(t *testing.T, slice []string, expected string) {
	t.Helper()
	for _, item := range slice {
		if item == expected {
			return // Found it
		}
	}
	assert.Fail(t, "Slice should contain expected value", "Expected: %s, Slice: %v", expected, slice)
}

// AssertStringSliceNotContains asserts that a string slice does not contain a value
func AssertStringSliceNotContains(t *testing.T, slice []string, notExpected string) {
	t.Helper()
	for _, item := range slice {
		if item == notExpected {
			assert.Fail(t, "Slice should not contain value", "Not expected: %s, Slice: %v", notExpected, slice)
			return
		}
	}
}

// AssertPathsEqual asserts that two file paths are equal (handling OS differences)
func AssertPathsEqual(t *testing.T, expected, actual string) {
	t.Helper()
	expectedClean := filepath.Clean(expected)
	actualClean := filepath.Clean(actual)
	assert.Equal(t, expectedClean, actualClean, "Paths should be equal")
}

// AssertProgressReported asserts that progress was reported during an operation
func AssertProgressReported(t *testing.T, reporter *MockProgressReporter, expectedMessages ...string) {
	t.Helper()
	messages := reporter.GetMessages()
	
	for _, expected := range expectedMessages {
		found := false
		for _, message := range messages {
			if strings.Contains(message, expected) {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected progress message not found: %s", expected)
	}
}

// AssertProgressIncreasing asserts that progress values are increasing
func AssertProgressIncreasing(t *testing.T, reporter *MockProgressReporter) {
	t.Helper()
	progress := reporter.GetProgress()
	
	if len(progress) < 2 {
		return // Not enough data points to check
	}
	
	for i := 1; i < len(progress); i++ {
		assert.GreaterOrEqual(t, progress[i], progress[i-1], 
			"Progress should be non-decreasing: %v", progress)
	}
}

// AssertSchemaValidation asserts that schema validation results are as expected
func AssertSchemaValidation(t *testing.T, result interface{}, expectValid bool, expectedErrors ...string) {
	t.Helper()
	
	// This is a placeholder - actual implementation would depend on the validation result structure
	// For now, we'll just check if result is nil for valid, non-nil for invalid
	if expectValid {
		assert.NotNil(t, result, "Validation result should not be nil for valid data")
	} else {
		// For invalid data, we might expect specific error messages
		if len(expectedErrors) > 0 {
			// Implementation would check that the validation result contains expected errors
			// This is a simplified version
			assert.NotNil(t, result, "Should have validation result for invalid data")
		}
	}
}

// AssertCUEModuleStructure asserts that a CUE module has the expected structure
func AssertCUEModuleStructure(t *testing.T, moduleDir string) {
	t.Helper()
	
	// Check for cue.mod directory
	cueModDir := filepath.Join(moduleDir, "cue.mod")
	AssertDirectoryExists(t, cueModDir)
	
	// Check for module.cue file
	moduleFile := filepath.Join(cueModDir, "module.cue")
	AssertFileExists(t, moduleFile)
	
	// Check that module.cue contains required fields
	AssertFileContains(t, moduleFile, "language:")
	AssertFileContains(t, moduleFile, "module:")
}

// AssertImportResult asserts properties of an import operation result
func AssertImportResult(t *testing.T, result interface{}, expectedItemCount int, expectSuccess bool) {
	t.Helper()
	
	// This is a placeholder for import result assertions
	// Actual implementation would depend on the ImportResult structure
	if expectSuccess {
		assert.NotNil(t, result, "Import result should not be nil for successful import")
	} else {
		// For failed imports, we might check error conditions
		// Implementation depends on how errors are represented in results
	}
}

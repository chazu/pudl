package testutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertFileExists asserts that a file exists at the given path
func AssertFileExists(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	assert.NoError(t, err, "File should exist: %s", path)
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

// AssertErrorContains asserts that an error contains the expected message
func AssertErrorContains(t *testing.T, err error, expectedMessage string) {
	t.Helper()
	require.Error(t, err, "Expected an error")
	assert.Contains(t, err.Error(), expectedMessage, "Error should contain expected message")
}

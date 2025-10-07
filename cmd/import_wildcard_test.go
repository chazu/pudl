package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFilePaths(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pudl-wildcard-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := []string{
		"data1.json",
		"data2.json", 
		"config.yaml",
		"logs.txt",
		"subdir/nested.json",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		
		// Create subdirectory if needed
		dir := filepath.Dir(fullPath)
		if dir != tempDir {
			err := os.MkdirAll(dir, 0755)
			require.NoError(t, err)
		}
		
		// Create the file
		f, err := os.Create(fullPath)
		require.NoError(t, err)
		f.WriteString(`{"test": "data"}`)
		f.Close()
	}

	// Change to temp directory for relative path tests
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)
	
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	t.Run("single file - no wildcard", func(t *testing.T) {
		paths, err := resolveFilePaths("data1.json")
		require.NoError(t, err)
		require.Len(t, paths, 1)
		assert.Contains(t, paths[0], "data1.json")
	})

	t.Run("wildcard - multiple JSON files", func(t *testing.T) {
		paths, err := resolveFilePaths("*.json")
		require.NoError(t, err)
		require.Len(t, paths, 2)
		
		// Should contain both JSON files
		fileNames := make([]string, len(paths))
		for i, path := range paths {
			fileNames[i] = filepath.Base(path)
		}
		assert.Contains(t, fileNames, "data1.json")
		assert.Contains(t, fileNames, "data2.json")
	})

	t.Run("wildcard - single YAML file", func(t *testing.T) {
		paths, err := resolveFilePaths("*.yaml")
		require.NoError(t, err)
		require.Len(t, paths, 1)
		assert.Contains(t, paths[0], "config.yaml")
	})

	t.Run("wildcard - no matches", func(t *testing.T) {
		paths, err := resolveFilePaths("*.xml")
		require.NoError(t, err)
		assert.Len(t, paths, 0)
	})

	t.Run("wildcard - subdirectory pattern", func(t *testing.T) {
		paths, err := resolveFilePaths("subdir/*.json")
		require.NoError(t, err)
		require.Len(t, paths, 1)
		assert.Contains(t, paths[0], "nested.json")
	})

	t.Run("single file - file not found", func(t *testing.T) {
		_, err := resolveFilePaths("nonexistent.json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("invalid wildcard pattern", func(t *testing.T) {
		_, err := resolveFilePaths("[invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid wildcard pattern")
	})
}

func TestContainsWildcard(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"data.json", false},
		{"*.json", true},
		{"data?.json", true},
		{"data[1-3].json", true},
		{"/path/to/file.txt", false},
		{"/path/*/file.txt", true},
		{"data/file?.yaml", true},
		{"normal-filename.csv", false},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			result := containsWildcard(test.path)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestResolveFilePathsFiltersDirectories(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pudl-wildcard-filter-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files and directories
	testFile := filepath.Join(tempDir, "data.json")
	f, err := os.Create(testFile)
	require.NoError(t, err)
	f.WriteString(`{"test": "data"}`)
	f.Close()

	// Create a directory that would match the pattern
	testDir := filepath.Join(tempDir, "data.json.dir")
	err = os.MkdirAll(testDir, 0755)
	require.NoError(t, err)

	// Change to temp directory
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)
	
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test that only files are returned, not directories
	paths, err := resolveFilePaths("data.json*")
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Contains(t, paths[0], "data.json")
	assert.NotContains(t, paths[0], "data.json.dir")
}

func TestWildcardImportIntegration(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pudl-wildcard-integration-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test JSON files with valid content
	testFiles := map[string]string{
		"data1.json": `{"name": "test1", "value": 100}`,
		"data2.json": `{"name": "test2", "value": 200}`,
		"config.yaml": `name: config\nvalue: 300`,
	}

	for filename, content := range testFiles {
		fullPath := filepath.Join(tempDir, filename)
		err := os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Change to temp directory
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	t.Run("wildcard pattern resolves correctly", func(t *testing.T) {
		// Test that wildcard pattern resolves to the correct files
		paths, err := resolveFilePaths("*.json")
		require.NoError(t, err)
		require.Len(t, paths, 2)

		// Check that both JSON files are included
		fileNames := make([]string, len(paths))
		for i, path := range paths {
			fileNames[i] = filepath.Base(path)
		}
		assert.Contains(t, fileNames, "data1.json")
		assert.Contains(t, fileNames, "data2.json")
		assert.NotContains(t, fileNames, "config.yaml")
	})

	t.Run("single file still works", func(t *testing.T) {
		// Test that single file import still works
		paths, err := resolveFilePaths("data1.json")
		require.NoError(t, err)
		require.Len(t, paths, 1)
		assert.Contains(t, paths[0], "data1.json")
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		// Test that patterns with no matches return empty slice
		paths, err := resolveFilePaths("*.xml")
		require.NoError(t, err)
		assert.Len(t, paths, 0)
	})
}

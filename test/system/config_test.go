package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/database"
	"pudl/internal/importer"
)

// SystemTestSuite provides system-level testing infrastructure
type SystemTestSuite struct {
	TempDir    string
	PUDLHome   string
	DataDir    string
	SchemaDir  string
	t          *testing.T
}

// NewSystemTestSuite creates a new system test suite
func NewSystemTestSuite(t *testing.T) *SystemTestSuite {
	tempDir := t.TempDir()
	
	suite := &SystemTestSuite{
		TempDir:   tempDir,
		PUDLHome:  filepath.Join(tempDir, ".pudl"),
		DataDir:   filepath.Join(tempDir, "data"),
		SchemaDir: filepath.Join(tempDir, "schemas"),
		t:         t,
	}
	
	return suite
}

// InitializeDirectories creates the required directory structure
func (s *SystemTestSuite) InitializeDirectories() error {
	dirs := []string{s.PUDLHome, s.DataDir, s.SchemaDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Create CUE module structure for schema directory
	cueModDir := filepath.Join(s.SchemaDir, "cue.mod")
	if err := os.MkdirAll(cueModDir, 0755); err != nil {
		return err
	}

	moduleContent := `language: version: "v0.14.0"
module: "pudl.schemas@v0"
source: kind: "self"
`
	if err := os.WriteFile(filepath.Join(cueModDir, "module.cue"), []byte(moduleContent), 0644); err != nil {
		return err
	}

	// Create core package with Item schema (universal fallback)
	coreDir := filepath.Join(s.SchemaDir, "pudl", "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		return err
	}

	coreContent := `package core

#Item: {
	_pudl: {
		schema_type:      "catchall"
		resource_type:    "unknown"
		cascade_priority: 0
		identity_fields: []
		tracked_fields: []
		compliance_level: "permissive"
	}
	...
}
`
	if err := os.WriteFile(filepath.Join(coreDir, "core.cue"), []byte(coreContent), 0644); err != nil {
		return err
	}

	return nil
}

func TestSystemConfiguration(t *testing.T) {
	suite := NewSystemTestSuite(t)
	require.NoError(t, suite.InitializeDirectories())

	t.Run("database initialization", func(t *testing.T) {
		// Test database can be initialized in various directory states
		testCases := []struct {
			name      string
			setupFunc func() string
			expectErr bool
		}{
			{
				name: "empty directory",
				setupFunc: func() string {
					return suite.PUDLHome
				},
				expectErr: false,
			},
			{
				name: "existing directory with files",
				setupFunc: func() string {
					dir := filepath.Join(suite.TempDir, "existing")
					os.MkdirAll(dir, 0755)
					// Create a dummy file
					os.WriteFile(filepath.Join(dir, "dummy.txt"), []byte("test"), 0644)
					return dir
				},
				expectErr: false,
			},
			{
				name: "nested directory creation",
				setupFunc: func() string {
					return filepath.Join(suite.TempDir, "deep", "nested", "path")
				},
				expectErr: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				dbPath := tc.setupFunc()
				
				db, err := database.NewCatalogDB(dbPath)
				
				if tc.expectErr {
					assert.Error(t, err, "Should fail for %s", tc.name)
					assert.Nil(t, db, "DB should be nil on error")
				} else {
					require.NoError(t, err, "Should succeed for %s", tc.name)
					require.NotNil(t, db, "DB should not be nil")
					
					// Verify database is functional
					result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
					require.NoError(t, err, "Query should work on new database")
					assert.Equal(t, 0, len(result.Entries), "New database should be empty")
					
					// Clean up
					err = db.Close()
					assert.NoError(t, err, "Should be able to close database")
				}
			})
		}
	})

	t.Run("importer initialization", func(t *testing.T) {
		// Test importer can be initialized with various configurations
		testCases := []struct {
			name      string
			dataDir   string
			schemaDir string
			pudelHome string
			expectErr bool
		}{
			{
				name:      "valid paths",
				dataDir:   suite.DataDir,
				schemaDir: suite.SchemaDir,
				pudelHome: suite.PUDLHome,
				expectErr: false,
			},
			{
				name:      "nonexistent data dir",
				dataDir:   filepath.Join(suite.TempDir, "nonexistent"),
				schemaDir: suite.SchemaDir,
				pudelHome: suite.PUDLHome,
				expectErr: false, // Should create directory
			},
			{
				name:      "empty paths",
				dataDir:   "",
				schemaDir: "",
				pudelHome: "",
				expectErr: true, // Schema path required for inference
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				imp, err := importer.New(tc.dataDir, tc.schemaDir, tc.pudelHome)
				
				if tc.expectErr {
					assert.Error(t, err, "Should fail for %s", tc.name)
					assert.Nil(t, imp, "Importer should be nil on error")
				} else {
					require.NoError(t, err, "Should succeed for %s", tc.name)
					require.NotNil(t, imp, "Importer should not be nil")
				}
			})
		}
	})

	t.Run("directory permissions", func(t *testing.T) {
		// Test behavior with various directory permissions
		testDir := filepath.Join(suite.TempDir, "permissions")
		os.MkdirAll(testDir, 0755)

		// Test read-only directory (if not running as root)
		if os.Getuid() != 0 {
			readOnlyDir := filepath.Join(testDir, "readonly")
			os.MkdirAll(readOnlyDir, 0444) // Read-only
			
			// Database creation should fail in read-only directory
			db, err := database.NewCatalogDB(readOnlyDir)
			assert.Error(t, err, "Should fail to create database in read-only directory")
			assert.Nil(t, db, "DB should be nil when creation fails")
			
			// Restore permissions for cleanup
			os.Chmod(readOnlyDir, 0755)
		}

		// Test writable directory
		writableDir := filepath.Join(testDir, "writable")
		os.MkdirAll(writableDir, 0755)
		
		db, err := database.NewCatalogDB(writableDir)
		require.NoError(t, err, "Should succeed in writable directory")
		require.NotNil(t, db, "DB should not be nil")
		
		err = db.Close()
		assert.NoError(t, err, "Should be able to close database")
	})

	t.Run("concurrent initialization", func(t *testing.T) {
		// Test concurrent database initialization
		const numConcurrent = 5
		results := make(chan error, numConcurrent)
		
		for i := 0; i < numConcurrent; i++ {
			go func(id int) {
				dbPath := filepath.Join(suite.TempDir, "concurrent", fmt.Sprintf("db%d", id))
				db, err := database.NewCatalogDB(dbPath)
				if err != nil {
					results <- err
					return
				}
				
				// Quick operation to verify functionality
				_, queryErr := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
				if queryErr != nil {
					results <- queryErr
					db.Close()
					return
				}
				
				closeErr := db.Close()
				results <- closeErr
			}(i)
		}
		
		// Collect results
		for i := 0; i < numConcurrent; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent initialization %d should succeed", i)
		}
	})
}

func TestSystemReliability(t *testing.T) {
	suite := NewSystemTestSuite(t)
	require.NoError(t, suite.InitializeDirectories())

	t.Run("database recovery", func(t *testing.T) {
		// Test database recovery after improper shutdown
		dbPath := filepath.Join(suite.TempDir, "recovery")
		
		// Create and populate database
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		
		// Add some test data
		testEntry := database.CatalogEntry{
			ID:              "recovery-test-001",
			StoredPath:      "/test/recovery.json",
			MetadataPath:    "/test/recovery.meta",
			ImportTimestamp: time.Now(),
			Format:          "json",
			Origin:          "recovery-test",
			Schema:          "test.#Recovery",
			Confidence:      0.9,
			RecordCount:     1,
			SizeBytes:       100,
		}
		
		err = db.AddEntry(testEntry)
		require.NoError(t, err)
		
		// Simulate improper shutdown (don't call Close())
		db = nil
		
		// Reopen database
		db2, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err, "Should be able to reopen database after improper shutdown")
		defer db2.Close()
		
		// Verify data is still there
		retrievedEntry, err := db2.GetEntry("recovery-test-001")
		require.NoError(t, err, "Should be able to retrieve entry after recovery")
		assert.Equal(t, testEntry.ID, retrievedEntry.ID, "Retrieved entry should match original")
	})

	t.Run("disk space handling", func(t *testing.T) {
		// Test behavior when disk space is limited
		// Note: This is a basic test - full disk simulation would require more complex setup
		
		dbPath := filepath.Join(suite.TempDir, "diskspace")
		db, err := database.NewCatalogDB(dbPath)
		require.NoError(t, err)
		defer db.Close()
		
		// Try to add many entries to test space usage
		const numEntries = 1000
		for i := 0; i < numEntries; i++ {
			entry := database.CatalogEntry{
				ID:              fmt.Sprintf("diskspace-test-%06d", i),
				StoredPath:      fmt.Sprintf("/test/diskspace-%06d.json", i),
				MetadataPath:    fmt.Sprintf("/test/diskspace-%06d.meta", i),
				ImportTimestamp: time.Now(),
				Format:          "json",
				Origin:          "diskspace-test",
				Schema:          "test.#DiskSpace",
				Confidence:      0.8,
				RecordCount:     1,
				SizeBytes:       int64(100 + i),
			}
			
			err = db.AddEntry(entry)
			if err != nil {
				// If we get an error, it should be a meaningful one
				t.Logf("Failed to add entry %d: %v", i, err)
				break
			}
		}
		
		// Database should still be functional
		result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
		require.NoError(t, err, "Database should remain functional")
		t.Logf("Successfully added %d entries", len(result.Entries))
	})

	t.Run("file system errors", func(t *testing.T) {
		// Test handling of various file system errors
		
		// Test with invalid characters in path (platform-specific)
		invalidPaths := []string{
			// These might be valid on some systems, so we test gracefully
			filepath.Join(suite.TempDir, "test\x00invalid"),
			filepath.Join(suite.TempDir, "test\x01invalid"),
		}
		
		for _, invalidPath := range invalidPaths {
			db, err := database.NewCatalogDB(invalidPath)
			if err != nil {
				// Error is expected and should be meaningful
				assert.NotEmpty(t, err.Error(), "Error should have descriptive message")
				assert.Nil(t, db, "DB should be nil on error")
				t.Logf("Invalid path correctly rejected: %s (%v)", invalidPath, err)
			} else {
				// If it succeeds, clean up
				if db != nil {
					db.Close()
				}
				t.Logf("Invalid path was accepted: %s", invalidPath)
			}
		}
	})
}

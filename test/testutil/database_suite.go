package testutil

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pudl/internal/database"
)

// Global cleanup registry for process-level safety
var (
	globalCleanupFunctions []func() error
	cleanupMutex          sync.Mutex
	signalHandlerOnce     sync.Once
)

func init() {
	// Register signal handlers for graceful cleanup on process termination
	signalHandlerOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		
		go func() {
			<-c
			log.Println("Test process interrupted, running global cleanup...")
			RunGlobalCleanup()
			os.Exit(1)
		}()
	})
}

// RegisterGlobalCleanup adds a cleanup function to the global registry
func RegisterGlobalCleanup(fn func() error) {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	globalCleanupFunctions = append(globalCleanupFunctions, fn)
}

// RunGlobalCleanup executes all registered global cleanup functions
func RunGlobalCleanup() {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	
	for i := len(globalCleanupFunctions) - 1; i >= 0; i-- {
		if err := globalCleanupFunctions[i](); err != nil {
			log.Printf("Global cleanup error: %v", err)
		}
	}
	globalCleanupFunctions = nil
}

// DatabaseTestSuite provides bulletproof database testing with guaranteed cleanup
type DatabaseTestSuite struct {
	TempDir    string
	DB         *database.CatalogDB
	cleanupFns []func() error
	t          *testing.T
}

// NewDatabaseTestSuite creates a new database test suite with automatic cleanup
func NewDatabaseTestSuite(t *testing.T) *DatabaseTestSuite {
	// Use t.TempDir() for automatic cleanup by Go's test runner
	tempDir := t.TempDir()
	
	suite := &DatabaseTestSuite{
		TempDir:    tempDir,
		cleanupFns: []func() error{},
		t:          t,
	}
	
	// Register cleanup that ALWAYS runs, even on panic/crash
	t.Cleanup(func() {
		suite.Cleanup()
	})
	
	// Register with global cleanup as additional safety net
	RegisterGlobalCleanup(func() error {
		suite.forceCleanup()
		return nil
	})
	
	return suite
}

// InitializeDatabase creates and initializes the test database
func (s *DatabaseTestSuite) InitializeDatabase() error {
	db, err := database.NewCatalogDB(s.TempDir)
	if err != nil {
		return fmt.Errorf("failed to create test database: %w", err)
	}
	
	s.DB = db
	
	// Register database cleanup
	s.RegisterCleanup(func() error {
		if s.DB != nil {
			return s.DB.Close()
		}
		return nil
	})
	
	return nil
}

// RegisterCleanup adds a cleanup function to be executed during teardown
func (s *DatabaseTestSuite) RegisterCleanup(fn func() error) {
	s.cleanupFns = append(s.cleanupFns, fn)
}

// Cleanup performs all registered cleanup operations
func (s *DatabaseTestSuite) Cleanup() {
	var cleanupErrors []error
	
	// Run all cleanup functions in reverse order (LIFO)
	for i := len(s.cleanupFns) - 1; i >= 0; i-- {
		if err := s.cleanupFns[i](); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	
	// Close database connections
	if s.DB != nil {
		if err := s.DB.Close(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		s.DB = nil
	}
	
	// Force cleanup any remaining files (belt and suspenders approach)
	if s.TempDir != "" {
		if err := os.RemoveAll(s.TempDir); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	
	// Log cleanup errors but don't fail the test
	for _, err := range cleanupErrors {
		if s.t != nil {
			s.t.Logf("Non-fatal cleanup error: %v", err)
		} else {
			log.Printf("Cleanup error: %v", err)
		}
	}
}

// forceCleanup performs cleanup without test context (for global cleanup)
func (s *DatabaseTestSuite) forceCleanup() {
	// Set t to nil to avoid test context issues during global cleanup
	originalT := s.t
	s.t = nil
	s.Cleanup()
	s.t = originalT
}

// SeedTestData populates the database with realistic test data
func (s *DatabaseTestSuite) SeedTestData() error {
	if s.DB == nil {
		return fmt.Errorf("database not initialized")
	}
	
	// Generate diverse test entries
	entries := []database.CatalogEntry{
		s.generateAWSEntry("aws-ec2-001", "aws.#EC2Instance"),
		s.generateAWSEntry("aws-s3-001", "aws.#S3Bucket"),
		s.generateK8sEntry("k8s-pod-001", "k8s.#Pod"),
		s.generateK8sEntry("k8s-svc-001", "k8s.#Service"),
		s.generateGenericEntry("generic-001", "unknown.#CatchAll"),
	}
	
	for _, entry := range entries {
		if err := s.DB.AddEntry(entry); err != nil {
			return fmt.Errorf("failed to seed test data: %w", err)
		}
	}
	
	return nil
}

// generateAWSEntry creates a realistic AWS catalog entry
func (s *DatabaseTestSuite) generateAWSEntry(id, schema string) database.CatalogEntry {
	now := time.Now()
	return database.CatalogEntry{
		ID:              id,
		StoredPath:      fmt.Sprintf("%s/raw/%s.json", s.TempDir, id),
		MetadataPath:    fmt.Sprintf("%s/metadata/%s.meta", s.TempDir, id),
		ImportTimestamp: now,
		Format:          "json",
		Origin:          "aws-ec2-describe-instances",
		Schema:          schema,
		Confidence:      0.9,
		RecordCount:     1,
		SizeBytes:       1024,
		CollectionID:    nil,
		ItemIndex:       nil,
		CollectionType:  nil,
		ItemID:          nil,
	}
}

// generateK8sEntry creates a realistic Kubernetes catalog entry
func (s *DatabaseTestSuite) generateK8sEntry(id, schema string) database.CatalogEntry {
	now := time.Now()
	return database.CatalogEntry{
		ID:              id,
		StoredPath:      fmt.Sprintf("%s/raw/%s.yaml", s.TempDir, id),
		MetadataPath:    fmt.Sprintf("%s/metadata/%s.meta", s.TempDir, id),
		ImportTimestamp: now,
		Format:          "yaml",
		Origin:          "k8s-get-pods",
		Schema:          schema,
		Confidence:      0.95,
		RecordCount:     1,
		SizeBytes:       512,
		CollectionID:    nil,
		ItemIndex:       nil,
		CollectionType:  nil,
		ItemID:          nil,
	}
}

// generateGenericEntry creates a generic catalog entry
func (s *DatabaseTestSuite) generateGenericEntry(id, schema string) database.CatalogEntry {
	now := time.Now()
	return database.CatalogEntry{
		ID:              id,
		StoredPath:      fmt.Sprintf("%s/raw/%s.json", s.TempDir, id),
		MetadataPath:    fmt.Sprintf("%s/metadata/%s.meta", s.TempDir, id),
		ImportTimestamp: now,
		Format:          "json",
		Origin:          "unknown",
		Schema:          schema,
		Confidence:      0.5,
		RecordCount:     1,
		SizeBytes:       256,
		CollectionID:    nil,
		ItemIndex:       nil,
		CollectionType:  nil,
		ItemID:          nil,
	}
}

// AssertQueryResults compares expected and actual query results
func (s *DatabaseTestSuite) AssertQueryResults(t *testing.T, expected, actual []database.CatalogEntry) {
	require.Equal(t, len(expected), len(actual), "Query result count mismatch")
	
	// Create maps for easier comparison
	expectedMap := make(map[string]database.CatalogEntry)
	actualMap := make(map[string]database.CatalogEntry)
	
	for _, entry := range expected {
		expectedMap[entry.ID] = entry
	}
	
	for _, entry := range actual {
		actualMap[entry.ID] = entry
	}
	
	// Compare each expected entry
	for id, expectedEntry := range expectedMap {
		actualEntry, exists := actualMap[id]
		require.True(t, exists, "Expected entry %s not found in results", id)
		
		// Compare key fields (allowing for minor timestamp differences)
		require.Equal(t, expectedEntry.ID, actualEntry.ID)
		require.Equal(t, expectedEntry.Schema, actualEntry.Schema)
		require.Equal(t, expectedEntry.Origin, actualEntry.Origin)
		require.Equal(t, expectedEntry.Format, actualEntry.Format)
	}
}

// CreateLargeDataset generates a large number of test entries for performance testing
func (s *DatabaseTestSuite) CreateLargeDataset(count int) ([]database.CatalogEntry, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	
	entries := make([]database.CatalogEntry, count)
	now := time.Now()
	
	for i := 0; i < count; i++ {
		// Distribute across different schemas and origins
		var schema, origin, format string
		switch i % 4 {
		case 0:
			schema = "aws.#EC2Instance"
			origin = "aws-ec2-describe-instances"
			format = "json"
		case 1:
			schema = "aws.#S3Bucket"
			origin = "aws-s3-list-buckets"
			format = "json"
		case 2:
			schema = "k8s.#Pod"
			origin = "k8s-get-pods"
			format = "yaml"
		case 3:
			schema = "k8s.#Service"
			origin = "k8s-get-services"
			format = "yaml"
		}
		
		entries[i] = database.CatalogEntry{
			ID:              fmt.Sprintf("large-dataset-%06d", i),
			StoredPath:      fmt.Sprintf("%s/raw/large-dataset-%06d.%s", s.TempDir, i, format),
			MetadataPath:    fmt.Sprintf("%s/metadata/large-dataset-%06d.meta", s.TempDir, i),
			ImportTimestamp: now.Add(time.Duration(i) * time.Second),
			Format:          format,
			Origin:          origin,
			Schema:          schema,
			Confidence:      0.8 + float64(i%20)/100.0, // Vary confidence
			RecordCount:     1 + i%10,                   // Vary record count
			SizeBytes:       int64(100 + i*10),          // Vary size
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries, nil
}

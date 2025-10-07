package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DatabaseTestSuite provides bulletproof database testing with guaranteed cleanup
type DatabaseTestSuite struct {
	TempDir    string
	DB         *CatalogDB
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
	
	return suite
}

// InitializeDatabase creates and initializes the test database
func (s *DatabaseTestSuite) InitializeDatabase() error {
	db, err := NewCatalogDB(s.TempDir)
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
	
	// Log cleanup errors but don't fail the test
	for _, err := range cleanupErrors {
		if s.t != nil {
			s.t.Logf("Non-fatal cleanup error: %v", err)
		}
	}
}

// TestDataGenerator provides methods to generate realistic test data
type TestDataGenerator struct {
	baseTime time.Time
	counter  int
}

// NewTestDataGenerator creates a new test data generator
func NewTestDataGenerator() *TestDataGenerator {
	return &TestDataGenerator{
		baseTime: time.Now().Add(-24 * time.Hour), // Start 24 hours ago
		counter:  0,
	}
}

// GenerateAWSEntries creates realistic AWS catalog entries
func (g *TestDataGenerator) GenerateAWSEntries(count int) []CatalogEntry {
	entries := make([]CatalogEntry, count)
	
	awsSchemas := []string{
		"aws.#EC2Instance",
		"aws.#S3Bucket",
		"aws.#RDSInstance",
		"aws.#SecurityGroup",
	}
	
	awsOrigins := []string{
		"aws-ec2-describe-instances",
		"aws-s3-list-buckets",
		"aws-rds-describe-db-instances",
		"aws-ec2-describe-security-groups",
	}
	
	for i := 0; i < count; i++ {
		schemaIndex := i % len(awsSchemas)
		g.counter++
		
		entries[i] = CatalogEntry{
			ID:              fmt.Sprintf("aws-test-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/aws-test-%06d.json", g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/aws-test-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute),
			Format:          "json",
			Origin:          awsOrigins[schemaIndex],
			Schema:          awsSchemas[schemaIndex],
			Confidence:      0.85 + float64(i%15)/100.0, // 0.85-0.99
			RecordCount:     1 + i%5,                     // 1-5 records
			SizeBytes:       int64(500 + i*50),           // Varying sizes
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateK8sEntries creates realistic Kubernetes catalog entries
func (g *TestDataGenerator) GenerateK8sEntries(count int) []CatalogEntry {
	entries := make([]CatalogEntry, count)
	
	k8sSchemas := []string{
		"k8s.#Pod",
		"k8s.#Service",
		"k8s.#Deployment",
		"k8s.#ConfigMap",
	}
	
	k8sOrigins := []string{
		"k8s-get-pods",
		"k8s-get-services",
		"k8s-get-deployments",
		"k8s-get-configmaps",
	}
	
	for i := 0; i < count; i++ {
		schemaIndex := i % len(k8sSchemas)
		g.counter++
		
		entries[i] = CatalogEntry{
			ID:              fmt.Sprintf("k8s-test-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/k8s-test-%06d.yaml", g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/k8s-test-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute * 2),
			Format:          "yaml",
			Origin:          k8sOrigins[schemaIndex],
			Schema:          k8sSchemas[schemaIndex],
			Confidence:      0.90 + float64(i%10)/100.0, // 0.90-0.99
			RecordCount:     1,                           // K8s resources are typically single objects
			SizeBytes:       int64(200 + i*25),           // Smaller than AWS resources
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateGenericEntries creates generic catalog entries for testing
func (g *TestDataGenerator) GenerateGenericEntries(count int) []CatalogEntry {
	entries := make([]CatalogEntry, count)
	
	genericSchemas := []string{
		"unknown.#CatchAll",
		"generic.#JSONData",
		"generic.#CSVData",
		"generic.#TextData",
	}
	
	genericOrigins := []string{
		"unknown",
		"manual-import",
		"csv-import",
		"text-import",
	}
	
	formats := []string{"json", "yaml", "csv", "txt"}
	
	for i := 0; i < count; i++ {
		schemaIndex := i % len(genericSchemas)
		formatIndex := i % len(formats)
		g.counter++
		
		entries[i] = CatalogEntry{
			ID:              fmt.Sprintf("generic-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/generic-%06d.%s", g.counter, formats[formatIndex]),
			MetadataPath:    fmt.Sprintf("/test/metadata/generic-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute * 3),
			Format:          formats[formatIndex],
			Origin:          genericOrigins[schemaIndex],
			Schema:          genericSchemas[schemaIndex],
			Confidence:      0.5 + float64(i%30)/100.0, // 0.5-0.79
			RecordCount:     1 + i%20,                   // 1-20 records
			SizeBytes:       int64(50 + i*15),           // Smaller files
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateMixedDataset creates a diverse dataset with different types of entries
func (g *TestDataGenerator) GenerateMixedDataset(totalCount int) []CatalogEntry {
	// Distribute entries across different types
	awsCount := totalCount * 40 / 100      // 40% AWS
	k8sCount := totalCount * 30 / 100      // 30% Kubernetes
	genericCount := totalCount - awsCount - k8sCount // Remaining generic
	
	var allEntries []CatalogEntry
	
	// Add AWS entries
	awsEntries := g.GenerateAWSEntries(awsCount)
	allEntries = append(allEntries, awsEntries...)
	
	// Add Kubernetes entries
	k8sEntries := g.GenerateK8sEntries(k8sCount)
	allEntries = append(allEntries, k8sEntries...)
	
	// Add generic entries
	genericEntries := g.GenerateGenericEntries(genericCount)
	allEntries = append(allEntries, genericEntries...)
	
	return allEntries
}

// GenerateCorruptedEntries creates entries with various data issues for error testing
func (g *TestDataGenerator) GenerateCorruptedEntries(count int) []CatalogEntry {
	entries := make([]CatalogEntry, count)
	
	for i := 0; i < count; i++ {
		g.counter++
		
		entry := CatalogEntry{
			ID:              fmt.Sprintf("corrupted-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/corrupted-%06d.json", g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/corrupted-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute),
			Format:          "json",
			Origin:          "corrupted-test",
			Schema:          "test.#CorruptedData",
			Confidence:      0.1, // Very low confidence
			RecordCount:     1,
			SizeBytes:       int64(50 + i*5),
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
		
		// Introduce various corruption patterns
		switch i % 4 {
		case 0:
			// Empty ID (should cause validation error)
			entry.ID = ""
		case 1:
			// Invalid confidence (outside 0-1 range)
			entry.Confidence = 1.5
		case 2:
			// Negative record count
			entry.RecordCount = -1
		case 3:
			// Zero timestamp
			entry.ImportTimestamp = time.Time{}
		}
		
		entries[i] = entry
	}
	
	return entries
}

// GenerateLargeDataset generates a large number of test entries for performance testing
func (g *TestDataGenerator) GenerateLargeDataset(count int) []CatalogEntry {
	entries := make([]CatalogEntry, count)
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
		
		g.counter++
		entries[i] = CatalogEntry{
			ID:              fmt.Sprintf("large-dataset-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/large-dataset-%06d.%s", g.counter, format),
			MetadataPath:    fmt.Sprintf("/test/metadata/large-dataset-%06d.meta", g.counter),
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
	
	return entries
}

// AssertDatabaseEntry validates a catalog entry against expected values
func AssertDatabaseEntry(t *testing.T, expected, actual *CatalogEntry) {
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
}

// AssertDatabaseEmpty checks if database is empty
func AssertDatabaseEmpty(t *testing.T, db *CatalogDB) {
	t.Helper()

	result, err := db.QueryEntries(FilterOptions{}, QueryOptions{})
	require.NoError(t, err, "Query should succeed")

	assert.Equal(t, 0, len(result.Entries), "Database should be empty")
	assert.Equal(t, 0, result.TotalCount, "Total count should be zero")
	assert.Equal(t, 0, result.FilteredCount, "Filtered count should be zero")
}

// AssertDatabaseCount checks if database has expected number of entries
func AssertDatabaseCount(t *testing.T, db *CatalogDB, expectedCount int) {
	t.Helper()

	result, err := db.QueryEntries(FilterOptions{}, QueryOptions{})
	require.NoError(t, err, "Query should succeed")

	assert.Equal(t, expectedCount, len(result.Entries), "Database should have %d entries", expectedCount)
	assert.Equal(t, expectedCount, result.TotalCount, "Total count should be %d", expectedCount)
	assert.Equal(t, expectedCount, result.FilteredCount, "Filtered count should be %d", expectedCount)
}

// AssertEntryExists checks if an entry exists in the database
func AssertEntryExists(t *testing.T, db *CatalogDB, entryID string) {
	t.Helper()

	entry, err := db.GetEntry(entryID)
	require.NoError(t, err, "GetEntry should succeed for existing entry")
	require.NotNil(t, entry, "Entry should exist")
	assert.Equal(t, entryID, entry.ID, "Retrieved entry should have correct ID")
}

// AssertEntryNotExists checks if an entry does not exist in the database
func AssertEntryNotExists(t *testing.T, db *CatalogDB, entryID string) {
	t.Helper()

	entry, err := db.GetEntry(entryID)
	assert.Error(t, err, "GetEntry should fail for non-existent entry")
	assert.Nil(t, entry, "Entry should not exist")
}

// AssertErrorContains checks if an error contains a specific substring
func AssertErrorContains(t *testing.T, err error, substring string) {
	t.Helper()
	require.Error(t, err, "Expected an error")
	assert.Contains(t, err.Error(), substring, "Error should contain '%s'", substring)
}

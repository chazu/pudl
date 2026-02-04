package infrastructure

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"pudl/internal/database"
	"pudl/internal/importer"
)

// Global cleanup registry for process-level safety
var (
	globalIntegrationCleanup []func() error
	integrationCleanupMutex  sync.Mutex
	integrationSignalOnce    sync.Once
)

func init() {
	// Register signal handlers for graceful cleanup on process termination
	integrationSignalOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		
		go func() {
			<-c
			log.Println("Integration test process interrupted, running cleanup...")
			RunGlobalIntegrationCleanup()
			os.Exit(1)
		}()
	})
}

// RegisterGlobalIntegrationCleanup adds a cleanup function to the global registry
func RegisterGlobalIntegrationCleanup(fn func() error) {
	integrationCleanupMutex.Lock()
	defer integrationCleanupMutex.Unlock()
	globalIntegrationCleanup = append(globalIntegrationCleanup, fn)
}

// RunGlobalIntegrationCleanup executes all registered global cleanup functions
func RunGlobalIntegrationCleanup() {
	integrationCleanupMutex.Lock()
	defer integrationCleanupMutex.Unlock()
	
	for i := len(globalIntegrationCleanup) - 1; i >= 0; i-- {
		if err := globalIntegrationCleanup[i](); err != nil {
			log.Printf("Global integration cleanup error: %v", err)
		}
	}
	globalIntegrationCleanup = nil
}

// IntegrationTestSuite provides comprehensive end-to-end testing infrastructure
type IntegrationTestSuite struct {
	// Workspace Management
	WorkspaceRoot string // Isolated test workspace (managed by t.TempDir())
	PUDLHome      string // PUDL configuration directory
	DataDir       string // Raw data storage directory
	SchemaDir     string // Schema definitions directory
	
	// Component Instances
	Importer    *importer.Importer    // File import engine
	Database    *database.CatalogDB   // Data catalog database
	
	// Test Data Management
	TestFiles     []string              // Track all created test files
	TestDataSets  map[string]*TestDataSet // Curated test datasets
	FileGenerator *TestFileGenerator    // Dynamic file creation
	
	// Cleanup and Safety
	cleanupFuncs []func() error        // Guaranteed cleanup functions
	t            *testing.T            // Test context for logging
	
	// Validation and Metrics
	Validators *IntegrationValidators  // End-to-end validation helpers
	Metrics    *TestMetrics           // Performance tracking
}

// TestDataSet represents a curated collection of test files
type TestDataSet struct {
	Name        string
	Description string
	Files       []TestFile
	Metadata    DataSetMetadata
}

// TestFile represents a single test data file
type TestFile struct {
	Name            string
	Content         string
	ExpectedRecords int
	ExpectedSchema  string
	ExpectedOrigin  string
	Format          string
}

// DataSetMetadata contains metadata about a test dataset
type DataSetMetadata struct {
	TotalFiles   int
	TotalRecords int
	TotalSize    int64
	Formats      []string
	Origins      []string
	Schemas      []string
}

// NewIntegrationTestSuite creates a new integration test suite with bulletproof cleanup
func NewIntegrationTestSuite(t *testing.T) *IntegrationTestSuite {
	// Use t.TempDir() for automatic cleanup by Go's test runner
	workspaceRoot := t.TempDir()
	
	suite := &IntegrationTestSuite{
		WorkspaceRoot: workspaceRoot,
		PUDLHome:      filepath.Join(workspaceRoot, ".pudl"),
		DataDir:       filepath.Join(workspaceRoot, "data"),
		SchemaDir:     filepath.Join(workspaceRoot, "schemas"),
		TestFiles:     []string{},
		TestDataSets:  make(map[string]*TestDataSet),
		cleanupFuncs:  []func() error{},
		t:             t,
	}
	
	// Register cleanup that ALWAYS runs, even on panic/crash
	t.Cleanup(func() {
		suite.Cleanup()
	})
	
	// Register with global cleanup as additional safety net
	RegisterGlobalIntegrationCleanup(func() error {
		suite.forceCleanup()
		return nil
	})
	
	return suite
}

// Initialize sets up the complete PUDL environment for testing
func (s *IntegrationTestSuite) Initialize() error {
	// Create directory structure
	dirs := []string{s.PUDLHome, s.DataDir, s.SchemaDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create CUE module structure for schema directory
	if err := s.initializeSchemaModule(); err != nil {
		return fmt.Errorf("failed to initialize schema module: %w", err)
	}

	// Initialize database
	db, err := database.NewCatalogDB(s.PUDLHome)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	s.Database = db

	// Initialize importer
	imp, err := importer.New(s.DataDir, s.SchemaDir, s.PUDLHome)
	if err != nil {
		return fmt.Errorf("failed to initialize importer: %w", err)
	}
	s.Importer = imp
	
	// Initialize test utilities
	s.FileGenerator = NewTestFileGenerator()
	s.Validators = NewIntegrationValidators(s)
	s.Metrics = NewTestMetrics()
	
	// Register component cleanup
	s.RegisterCleanup(func() error {
		var errors []error
		
		if s.Importer != nil {
			// Importer doesn't have a Close method, just nil it
			s.Importer = nil
		}
		
		if s.Database != nil {
			if err := s.Database.Close(); err != nil {
				errors = append(errors, err)
			}
			s.Database = nil
		}
		
		if len(errors) > 0 {
			return fmt.Errorf("component cleanup errors: %v", errors)
		}
		return nil
	})
	
	return nil
}

// RegisterCleanup adds a cleanup function to be executed during teardown
func (s *IntegrationTestSuite) RegisterCleanup(fn func() error) {
	s.cleanupFuncs = append(s.cleanupFuncs, fn)
}

// CreateTestFile creates a test file and tracks it for cleanup
func (s *IntegrationTestSuite) CreateTestFile(name, content string) (string, error) {
	filePath := filepath.Join(s.DataDir, name)
	
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	
	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}
	
	// Track for cleanup
	s.TestFiles = append(s.TestFiles, filePath)
	
	return filePath, nil
}

// LoadTestDataSet loads a curated test dataset
func (s *IntegrationTestSuite) LoadTestDataSet(name string) (*TestDataSet, error) {
	dataSet, exists := GetTestDataSet(name)
	if !exists {
		return nil, fmt.Errorf("test dataset %s not found", name)
	}
	
	// Create all files in the dataset
	for i, file := range dataSet.Files {
		filePath, err := s.CreateTestFile(file.Name, file.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to create test file %s: %w", file.Name, err)
		}
		
		// Update file path in dataset
		dataSet.Files[i].Name = filePath
	}
	
	s.TestDataSets[name] = dataSet
	return dataSet, nil
}

// ImportTestDataSet imports all files from a test dataset
func (s *IntegrationTestSuite) ImportTestDataSet(dataSetName string) ([]*importer.ImportResult, error) {
	dataSet, exists := s.TestDataSets[dataSetName]
	if !exists {
		return nil, fmt.Errorf("test dataset %s not loaded", dataSetName)
	}
	
	var results []*importer.ImportResult
	
	for _, file := range dataSet.Files {
		opts := importer.ImportOptions{
			SourcePath: file.Name,
		}
		
		result, err := s.Importer.ImportFile(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to import file %s: %w", file.Name, err)
		}
		
		results = append(results, result)
	}
	
	return results, nil
}

// GetDatabaseEntryCount returns the total number of entries in the database
func (s *IntegrationTestSuite) GetDatabaseEntryCount() (int, error) {
	result, err := s.Database.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if err != nil {
		return 0, err
	}
	return result.TotalCount, nil
}

// QueryDatabase performs a database query with the given filters
func (s *IntegrationTestSuite) QueryDatabase(filters database.FilterOptions, options database.QueryOptions) (*database.QueryResult, error) {
	return s.Database.QueryEntries(filters, options)
}

// Cleanup performs all registered cleanup operations
func (s *IntegrationTestSuite) Cleanup() {
	var cleanupErrors []error
	
	// Run all cleanup functions in reverse order (LIFO)
	for i := len(s.cleanupFuncs) - 1; i >= 0; i-- {
		if err := s.cleanupFuncs[i](); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	
	// Clean up test files (redundant with t.TempDir but safe)
	for _, file := range s.TestFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	
	// Clean up workspace (redundant with t.TempDir but safe)
	if s.WorkspaceRoot != "" {
		if err := os.RemoveAll(s.WorkspaceRoot); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	
	// Log cleanup errors but don't fail the test
	for _, err := range cleanupErrors {
		if s.t != nil {
			s.t.Logf("Non-fatal integration cleanup error: %v", err)
		} else {
			log.Printf("Integration cleanup error: %v", err)
		}
	}
}

// forceCleanup performs cleanup without test context (for global cleanup)
func (s *IntegrationTestSuite) forceCleanup() {
	// Set t to nil to avoid test context issues during global cleanup
	originalT := s.t
	s.t = nil
	s.Cleanup()
	s.t = originalT
}

// initializeSchemaModule creates the CUE module structure with bootstrap schemas
func (s *IntegrationTestSuite) initializeSchemaModule() error {
	// Create cue.mod directory
	cueModDir := filepath.Join(s.SchemaDir, "cue.mod")
	if err := os.MkdirAll(cueModDir, 0755); err != nil {
		return fmt.Errorf("failed to create cue.mod: %w", err)
	}

	// Create module.cue
	moduleContent := `language: version: "v0.14.0"
module: "pudl.schemas@v0"
source: kind: "self"
`
	if err := os.WriteFile(filepath.Join(cueModDir, "module.cue"), []byte(moduleContent), 0644); err != nil {
		return fmt.Errorf("failed to write module.cue: %w", err)
	}

	// Create core package with catchall schema
	coreDir := filepath.Join(s.SchemaDir, "pudl", "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		return fmt.Errorf("failed to create core package: %w", err)
	}

	coreContent := `package core

#CatchAll: {
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
		return fmt.Errorf("failed to write core.cue: %w", err)
	}

	return nil
}

// LogInfo logs an informational message
func (s *IntegrationTestSuite) LogInfo(format string, args ...interface{}) {
	if s.t != nil {
		s.t.Logf("[INFO] "+format, args...)
	} else {
		log.Printf("[INFO] "+format, args...)
	}
}

// LogError logs an error message
func (s *IntegrationTestSuite) LogError(format string, args ...interface{}) {
	if s.t != nil {
		s.t.Errorf("[ERROR] "+format, args...)
	} else {
		log.Printf("[ERROR] "+format, args...)
	}
}

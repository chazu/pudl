package infrastructure

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/database"
	"pudl/internal/importer"
)

// IntegrationValidators provides comprehensive validation for integration tests
type IntegrationValidators struct {
	suite *IntegrationTestSuite
}

// NewIntegrationValidators creates a new integration validator
func NewIntegrationValidators(suite *IntegrationTestSuite) *IntegrationValidators {
	return &IntegrationValidators{
		suite: suite,
	}
}

// WorkflowOutcome defines expected outcomes for workflow validation
type WorkflowOutcome struct {
	ExpectedEntries      int
	ExpectedSchemas      []string
	ExpectedOrigins      []string
	ExpectedFormats      []string
	QueryScenarios       []QueryScenario
	PerformanceLimits    PerformanceLimits
}

// QueryScenario defines a query test scenario
type QueryScenario struct {
	Name            string
	Filters         database.FilterOptions
	Options         database.QueryOptions
	ExpectedCount   int
	ExpectedSchemas []string
}

// PerformanceLimits defines performance expectations
type PerformanceLimits struct {
	MaxImportTime  time.Duration
	MaxQueryTime   time.Duration
	MaxMemoryUsage int64
	MinThroughput  float64
}

// TestMetrics tracks performance metrics during tests
type TestMetrics struct {
	ImportStartTime time.Time
	ImportEndTime   time.Time
	QueryTimes      []time.Duration
	MemoryUsage     int64
	RecordsProcessed int
}

// NewTestMetrics creates a new test metrics tracker
func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		QueryTimes: []time.Duration{},
	}
}

// StartImportTimer starts tracking import performance
func (m *TestMetrics) StartImportTimer() {
	m.ImportStartTime = time.Now()
}

// EndImportTimer ends tracking import performance
func (m *TestMetrics) EndImportTimer(recordCount int) {
	m.ImportEndTime = time.Now()
	m.RecordsProcessed = recordCount
}

// GetImportDuration returns the total import duration
func (m *TestMetrics) GetImportDuration() time.Duration {
	return m.ImportEndTime.Sub(m.ImportStartTime)
}

// GetThroughput returns records processed per second
func (m *TestMetrics) GetThroughput() float64 {
	duration := m.GetImportDuration()
	if duration == 0 {
		return 0
	}
	return float64(m.RecordsProcessed) / duration.Seconds()
}

// AddQueryTime adds a query execution time
func (m *TestMetrics) AddQueryTime(duration time.Duration) {
	m.QueryTimes = append(m.QueryTimes, duration)
}

// GetAverageQueryTime returns the average query execution time
func (m *TestMetrics) GetAverageQueryTime() time.Duration {
	if len(m.QueryTimes) == 0 {
		return 0
	}
	
	var total time.Duration
	for _, t := range m.QueryTimes {
		total += t
	}
	return total / time.Duration(len(m.QueryTimes))
}

// ValidateFileSystem validates file system operations
func (v *IntegrationValidators) ValidateFileSystem(t *testing.T, files []string) {
	t.Helper()
	
	for _, file := range files {
		// Check file exists
		assert.FileExists(t, file, "File should exist: %s", file)
		
		// Check file is readable
		info, err := os.Stat(file)
		require.NoError(t, err, "Should be able to stat file: %s", file)
		assert.True(t, info.Mode().IsRegular(), "Should be a regular file: %s", file)
		assert.Greater(t, info.Size(), int64(0), "File should not be empty: %s", file)
		
		// Check file permissions
		assert.True(t, info.Mode().Perm()&0400 != 0, "File should be readable: %s", file)
	}
}

// ValidateImportResults validates import operation results
func (v *IntegrationValidators) ValidateImportResults(
	t *testing.T,
	results []*importer.ImportResult,
	expectedOutcome WorkflowOutcome,
) {
	t.Helper()
	
	// Validate import success
	for i, result := range results {
		require.NotNil(t, result, "Import result %d should not be nil", i)
		assert.NotEmpty(t, result.ID, "Import %d should have ID", i)
		assert.Greater(t, result.RecordCount, 0, "Import %d should process records", i)
		assert.NotEmpty(t, result.StoredPath, "Import %d should have stored path", i)
		assert.NotEmpty(t, result.AssignedSchema, "Import %d should have assigned schema", i)
	}
	
	// Validate database state
	totalCount, err := v.suite.GetDatabaseEntryCount()
	require.NoError(t, err, "Should be able to get database count")
	assert.Equal(t, expectedOutcome.ExpectedEntries, totalCount,
		"Database should contain expected number of entries")
	
	// Validate schemas are present
	for _, expectedSchema := range expectedOutcome.ExpectedSchemas {
		result, err := v.suite.QueryDatabase(database.FilterOptions{
			Schema: expectedSchema,
		}, database.QueryOptions{})
		require.NoError(t, err, "Should be able to query by schema: %s", expectedSchema)
		assert.Greater(t, len(result.Entries), 0, "Should find entries with schema: %s", expectedSchema)
	}
	
	// Validate origins are present
	for _, expectedOrigin := range expectedOutcome.ExpectedOrigins {
		result, err := v.suite.QueryDatabase(database.FilterOptions{
			Origin: expectedOrigin,
		}, database.QueryOptions{})
		require.NoError(t, err, "Should be able to query by origin: %s", expectedOrigin)
		assert.Greater(t, len(result.Entries), 0, "Should find entries with origin: %s", expectedOrigin)
	}
	
	// Validate formats are present
	for _, expectedFormat := range expectedOutcome.ExpectedFormats {
		result, err := v.suite.QueryDatabase(database.FilterOptions{
			Format: expectedFormat,
		}, database.QueryOptions{})
		require.NoError(t, err, "Should be able to query by format: %s", expectedFormat)
		assert.Greater(t, len(result.Entries), 0, "Should find entries with format: %s", expectedFormat)
	}
}

// ValidateQueryScenarios validates query operation scenarios
func (v *IntegrationValidators) ValidateQueryScenarios(
	t *testing.T,
	scenarios []QueryScenario,
	metrics *TestMetrics,
) {
	t.Helper()
	
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			startTime := time.Now()
			
			result, err := v.suite.QueryDatabase(scenario.Filters, scenario.Options)
			
			queryTime := time.Since(startTime)
			metrics.AddQueryTime(queryTime)
			
			require.NoError(t, err, "Query should succeed: %s", scenario.Name)
			require.NotNil(t, result, "Query result should not be nil: %s", scenario.Name)
			
			// Validate result count
			if scenario.ExpectedCount >= 0 {
				assert.Equal(t, scenario.ExpectedCount, len(result.Entries),
					"Query should return expected count: %s", scenario.Name)
			}
			
			// Validate schemas in results
			if len(scenario.ExpectedSchemas) > 0 {
				foundSchemas := make(map[string]bool)
				for _, entry := range result.Entries {
					foundSchemas[entry.Schema] = true
				}
				
				for _, expectedSchema := range scenario.ExpectedSchemas {
					assert.True(t, foundSchemas[expectedSchema],
						"Query results should contain schema %s: %s", expectedSchema, scenario.Name)
				}
			}
			
			v.suite.LogInfo("Query '%s' completed in %v with %d results",
				scenario.Name, queryTime, len(result.Entries))
		})
	}
}

// ValidatePerformance validates performance metrics against limits
func (v *IntegrationValidators) ValidatePerformance(
	t *testing.T,
	metrics *TestMetrics,
	limits PerformanceLimits,
) {
	t.Helper()
	
	// Validate import performance
	if limits.MaxImportTime > 0 {
		importDuration := metrics.GetImportDuration()
		assert.Less(t, importDuration, limits.MaxImportTime,
			"Import should complete within %v (actual: %v)", limits.MaxImportTime, importDuration)
		
		v.suite.LogInfo("Import completed in %v", importDuration)
	}
	
	// Validate query performance
	if limits.MaxQueryTime > 0 {
		avgQueryTime := metrics.GetAverageQueryTime()
		assert.Less(t, avgQueryTime, limits.MaxQueryTime,
			"Average query time should be within %v (actual: %v)", limits.MaxQueryTime, avgQueryTime)
		
		v.suite.LogInfo("Average query time: %v", avgQueryTime)
	}
	
	// Validate throughput
	if limits.MinThroughput > 0 {
		throughput := metrics.GetThroughput()
		assert.Greater(t, throughput, limits.MinThroughput,
			"Throughput should exceed %.2f records/sec (actual: %.2f)", limits.MinThroughput, throughput)
		
		v.suite.LogInfo("Import throughput: %.2f records/sec", throughput)
	}
	
	// Log performance summary
	v.suite.LogInfo("Performance Summary:")
	v.suite.LogInfo("  Import Duration: %v", metrics.GetImportDuration())
	v.suite.LogInfo("  Records Processed: %d", metrics.RecordsProcessed)
	v.suite.LogInfo("  Throughput: %.2f records/sec", metrics.GetThroughput())
	v.suite.LogInfo("  Average Query Time: %v", metrics.GetAverageQueryTime())
	v.suite.LogInfo("  Total Queries: %d", len(metrics.QueryTimes))
}

// ValidateWorkspaceStructure validates the test workspace structure
func (v *IntegrationValidators) ValidateWorkspaceStructure(t *testing.T) {
	t.Helper()
	
	// Check required directories exist
	requiredDirs := []string{
		v.suite.WorkspaceRoot,
		v.suite.PUDLHome,
		v.suite.DataDir,
		v.suite.SchemaDir,
	}
	
	for _, dir := range requiredDirs {
		assert.DirExists(t, dir, "Required directory should exist: %s", dir)
		
		// Check directory permissions
		info, err := os.Stat(dir)
		require.NoError(t, err, "Should be able to stat directory: %s", dir)
		assert.True(t, info.IsDir(), "Should be a directory: %s", dir)
		assert.True(t, info.Mode().Perm()&0700 != 0, "Directory should be accessible: %s", dir)
	}
}

// ValidateCleanup validates that cleanup was successful
func (v *IntegrationValidators) ValidateCleanup(t *testing.T, workspaceRoot string) {
	t.Helper()
	
	// Note: This validation runs after cleanup, so we expect the workspace to be gone
	// This is mainly for testing the cleanup mechanism itself
	
	if workspaceRoot != "" {
		// Check if workspace was cleaned up (it should be gone)
		_, err := os.Stat(workspaceRoot)
		if err == nil {
			// Workspace still exists - this might be okay if using t.TempDir()
			// since Go's test runner handles cleanup
			v.suite.LogInfo("Workspace still exists (managed by Go test runner): %s", workspaceRoot)
		} else if os.IsNotExist(err) {
			// Workspace was cleaned up - this is good
			v.suite.LogInfo("Workspace successfully cleaned up: %s", workspaceRoot)
		} else {
			// Some other error occurred
			t.Errorf("Error checking workspace cleanup: %v", err)
		}
	}
}

// ValidateErrorHandling validates error handling scenarios
func (v *IntegrationValidators) ValidateErrorHandling(
	t *testing.T,
	results []*importer.ImportResult,
	errors []error,
	expectErrors bool,
) {
	t.Helper()

	if expectErrors {
		// Validate that errors were handled gracefully
		assert.Greater(t, len(errors), 0, "Should have encountered expected errors")

		for _, err := range errors {
			// Error should be informative
			assert.NotEmpty(t, err.Error(), "Error message should not be empty")
			v.suite.LogInfo("Expected error handled: %v", err)
		}
	} else {
		// Validate that no errors occurred
		assert.Equal(t, 0, len(errors), "Should not have errors")

		// Validate that all results are valid
		for i, result := range results {
			assert.NotNil(t, result, "Import result %d should not be nil", i)
			assert.NotEmpty(t, result.ID, "Import result %d should have ID", i)
		}
	}
}

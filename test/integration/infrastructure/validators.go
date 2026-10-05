package infrastructure

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/importer"
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
	ExpectedEntries   int
	ExpectedSchemas   []string
	ExpectedOrigins   []string
	ExpectedFormats   []string
	QueryScenarios    []QueryScenario
	PerformanceLimits PerformanceLimits
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
	ImportStartTime  time.Time
	ImportEndTime    time.Time
	QueryTimes       []time.Duration
	MemoryUsage      int64
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

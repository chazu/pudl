package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/test/integration/infrastructure"
)

func TestIntegrationInfrastructure(t *testing.T) {
	t.Run("test suite initialization", func(t *testing.T) {
		suite := infrastructure.NewIntegrationTestSuite(t)
		require.NotNil(t, suite)
		
		// Test workspace structure
		assert.NotEmpty(t, suite.WorkspaceRoot)
		assert.NotEmpty(t, suite.PUDLHome)
		assert.NotEmpty(t, suite.DataDir)
		assert.NotEmpty(t, suite.SchemaDir)
		
		// Initialize the suite
		err := suite.Initialize()
		require.NoError(t, err)
		
		// Validate workspace structure
		suite.Validators.ValidateWorkspaceStructure(t)
		
		// Test components are initialized
		assert.NotNil(t, suite.Database)
		assert.NotNil(t, suite.Importer)
		assert.NotNil(t, suite.FileGenerator)
		assert.NotNil(t, suite.Validators)
		assert.NotNil(t, suite.Metrics)
	})
	
	t.Run("test data generation", func(t *testing.T) {
		suite := infrastructure.NewIntegrationTestSuite(t)
		require.NoError(t, suite.Initialize())
		
		// Test AWS data generation
		awsData := suite.FileGenerator.GenerateAWSEC2Response(5)
		assert.NotEmpty(t, awsData)
		assert.Contains(t, awsData, "InstanceId")
		assert.Contains(t, awsData, "Reservations")
		
		// Test Kubernetes data generation
		k8sData := suite.FileGenerator.GenerateKubernetesPods(3)
		assert.NotEmpty(t, k8sData)
		assert.Contains(t, k8sData, "apiVersion")
		assert.Contains(t, k8sData, "kind")
		
		// Test large data generation
		largeData := suite.FileGenerator.GenerateLargeNDJSON(100)
		assert.NotEmpty(t, largeData)
		// Should have 100 lines (one per record)
		lines := len(strings.Split(largeData, "\n"))
		assert.Equal(t, 100, lines)
	})
	
	t.Run("test dataset loading", func(t *testing.T) {
		suite := infrastructure.NewIntegrationTestSuite(t)
		require.NoError(t, suite.Initialize())
		
		// Test loading minimal dataset
		dataset, err := suite.LoadTestDataSet("minimal-test")
		require.NoError(t, err)
		require.NotNil(t, dataset)
		
		assert.Equal(t, "Minimal Test Dataset", dataset.Name)
		assert.Equal(t, 2, len(dataset.Files))
		assert.Equal(t, 2, dataset.Metadata.TotalRecords)
		
		// Validate files were created
		for _, file := range dataset.Files {
			assert.FileExists(t, file.Name)
		}
	})
	
	t.Run("test file creation and tracking", func(t *testing.T) {
		suite := infrastructure.NewIntegrationTestSuite(t)
		require.NoError(t, suite.Initialize())
		
		// Create test file
		content := `{"test": "data", "id": 1}`
		filePath, err := suite.CreateTestFile("test.json", content)
		require.NoError(t, err)
		
		// Validate file exists and is tracked
		assert.FileExists(t, filePath)
		assert.Contains(t, suite.TestFiles, filePath)
		
		// Validate file content
		suite.Validators.ValidateFileSystem(t, []string{filePath})
	})
	
	t.Run("test available datasets", func(t *testing.T) {
		datasets := infrastructure.ListAvailableDataSets()
		assert.Greater(t, len(datasets), 0)
		
		expectedDatasets := []string{
			"aws-production-sample",
			"k8s-cluster-snapshot",
			"mixed-environment",
			"large-dataset",
			"corrupted-data",
			"minimal-test",
		}
		
		for _, expected := range expectedDatasets {
			assert.Contains(t, datasets, expected)
		}
		
		// Test dataset info
		for _, name := range datasets {
			info := infrastructure.GetDataSetInfo(name)
			assert.NotEmpty(t, info)
			// Info contains the dataset description, not the key name
			assert.Contains(t, info, "Dataset:")
		}
	})
	
	t.Run("test metrics tracking", func(t *testing.T) {
		metrics := infrastructure.NewTestMetrics()
		require.NotNil(t, metrics)
		
		// Test import timing
		metrics.StartImportTimer()
		time.Sleep(10 * time.Millisecond) // Simulate work
		metrics.EndImportTimer(100)
		
		duration := metrics.GetImportDuration()
		assert.Greater(t, duration, 10*time.Millisecond)
		
		throughput := metrics.GetThroughput()
		assert.Greater(t, throughput, 0.0)
		
		// Test query timing
		metrics.AddQueryTime(5 * time.Millisecond)
		metrics.AddQueryTime(10 * time.Millisecond)
		
		avgTime := metrics.GetAverageQueryTime()
		assert.Equal(t, 7500*time.Microsecond, avgTime) // (5ms + 10ms) / 2 = 7.5ms
	})
}

func TestDataSetGeneration(t *testing.T) {
	t.Run("aws production sample", func(t *testing.T) {
		dataset, exists := infrastructure.GetTestDataSet("aws-production-sample")
		require.True(t, exists)
		require.NotNil(t, dataset)
		
		assert.Equal(t, "AWS Production Environment Sample", dataset.Name)
		assert.Equal(t, 2, len(dataset.Files))
		assert.Equal(t, 23, dataset.Metadata.TotalRecords)
		
		// Validate file contents are realistic
		for _, file := range dataset.Files {
			assert.NotEmpty(t, file.Content)
			assert.Greater(t, file.ExpectedRecords, 0)
			assert.NotEmpty(t, file.ExpectedSchema)
			assert.NotEmpty(t, file.ExpectedOrigin)
		}
	})
	
	t.Run("k8s cluster snapshot", func(t *testing.T) {
		dataset, exists := infrastructure.GetTestDataSet("k8s-cluster-snapshot")
		require.True(t, exists)
		require.NotNil(t, dataset)
		
		assert.Equal(t, "Kubernetes Cluster State Snapshot", dataset.Name)
		assert.Equal(t, 2, len(dataset.Files))
		assert.Equal(t, 35, dataset.Metadata.TotalRecords)
		
		// Validate YAML content
		for _, file := range dataset.Files {
			if file.Format == "yaml" {
				assert.Contains(t, file.Content, "apiVersion")
				assert.Contains(t, file.Content, "kind")
			}
		}
	})
	
	t.Run("corrupted data", func(t *testing.T) {
		dataset, exists := infrastructure.GetTestDataSet("corrupted-data")
		require.True(t, exists)
		require.NotNil(t, dataset)
		
		assert.Equal(t, "Corrupted Data Test Dataset", dataset.Name)
		assert.Equal(t, 4, len(dataset.Files))
		assert.Equal(t, 0, dataset.Metadata.TotalRecords) // All should fail
		
		// Validate corruption types
		corruptionTypes := []string{"missing-brace", "invalid-json", "truncated", "invalid-utf8"}
		for i, file := range dataset.Files {
			assert.Contains(t, file.Name, corruptionTypes[i])
			assert.Equal(t, 0, file.ExpectedRecords) // Should fail to parse
		}
	})
}

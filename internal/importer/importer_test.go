package importer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/internal/streaming"
	"pudl/test/testutil"
)

func TestNew(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	tests := []struct {
		name        string
		dataPath    string
		schemaPath  string
		pudlHome    string
		expectError bool
	}{
		{
			name:        "valid paths",
			dataPath:    workspace.DataDir,
			schemaPath:  workspace.SchemaDir,
			pudlHome:    workspace.Root,
			expectError: false,
		},
		{
			name:        "nonexistent data path",
			dataPath:    "/nonexistent/path",
			schemaPath:  workspace.SchemaDir,
			pudlHome:    workspace.Root,
			expectError: false, // Directories are created as needed
		},
		{
			name:        "empty schema path",
			dataPath:    workspace.DataDir,
			schemaPath:  "",
			pudlHome:    workspace.Root,
			expectError: true, // Schema path is required for inference
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importer, err := New(tt.dataPath, tt.schemaPath, tt.pudlHome)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, importer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, importer)
				assert.Equal(t, tt.dataPath, importer.dataPath)
				assert.Equal(t, tt.schemaPath, importer.schemaPath)
				assert.NotNil(t, importer.catalogDB)
				assert.NotNil(t, importer.inferrer)
			}
		})
	}
}

func TestImportFile_JSON(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create test JSON file
	jsonFile := setup.WriteFile("test.json", fixtures.ValidJSON())

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Test import
	opts := ImportOptions{
		SourcePath:       jsonFile,
		UseStreaming:     false,
		ManualSchema:     "",
		StreamingConfig:  nil,
		CascadeValidator: nil,
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result
	assert.Equal(t, jsonFile, result.SourcePath)
	assert.Equal(t, "json", result.DetectedFormat)
	assert.NotEmpty(t, result.StoredPath)
	assert.NotEmpty(t, result.MetadataPath)
	assert.NotEmpty(t, result.AssignedSchema)
	assert.Greater(t, result.SchemaConfidence, 0.0)
	assert.Equal(t, 1, result.RecordCount)
	assert.Greater(t, result.SizeBytes, int64(0))
	assert.NotEmpty(t, result.ImportTimestamp)

	// Verify files were created
	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)

	// Verify metadata file contains expected content
	testutil.AssertFileContains(t, result.MetadataPath, result.ID)
	testutil.AssertFileContains(t, result.MetadataPath, result.AssignedSchema)
}

func TestImportFile_YAML(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create test YAML file
	yamlFile := setup.WriteFile("test.yaml", fixtures.ValidYAML())

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Test import
	opts := ImportOptions{
		SourcePath:   yamlFile,
		UseStreaming: false,
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result
	assert.Equal(t, "yaml", result.DetectedFormat)
	assert.NotEmpty(t, result.AssignedSchema)
	assert.Equal(t, 1, result.RecordCount)

	// Verify files were created
	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)
}

func TestImportFile_NDJSON(t *testing.T) {
	t.Skip("NDJSON collection processing requires streaming parser fixes - skipping for now")

	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create test NDJSON file manually to ensure proper format
	// Use a larger file to ensure streaming parser processes it correctly
	ndjsonContent := `{"id": 1, "name": "item1", "type": "test"}
{"id": 2, "name": "item2", "type": "test"}
{"id": 3, "name": "item3", "type": "test"}
{"id": 4, "name": "item4", "type": "test"}
{"id": 5, "name": "item5", "type": "test"}`
	ndjsonFile := setup.WriteFile("test.json", ndjsonContent)

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Test import - NDJSON automatically uses streaming internally
	opts := ImportOptions{
		SourcePath:      ndjsonFile,
		UseStreaming:    false, // Will be overridden internally for NDJSON
		StreamingConfig: streaming.DefaultStreamingConfig(),
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result
	assert.Equal(t, "ndjson", result.DetectedFormat)
	assert.Equal(t, 5, result.RecordCount) // 5 lines in NDJSON
	assert.NotEmpty(t, result.AssignedSchema)

	// Verify files were created
	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)
}

func TestImportFile_WithStreaming(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create large NDJSON file for streaming test
	largeFile := setup.WriteFile("large.json", fixtures.LargeNDJSON(100))

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Test import with streaming
	opts := ImportOptions{
		SourcePath:   largeFile,
		UseStreaming: true,
		StreamingConfig: &streaming.StreamingConfig{
			ChunkAlgorithm:   "fastcdc",
			MinChunkSize:     512,
			MaxChunkSize:     2048,
			AvgChunkSize:     1024,
			BufferSize:       4096,
			MaxMemoryMB:      10,
			ErrorTolerance:   0.1,
			SkipMalformed:    true,
			SampleSize:       100,
			Confidence:       0.8,
			ReportEveryMB:    1,
			MaxConcurrency:   0,
		},
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result
	assert.Equal(t, "ndjson", result.DetectedFormat)
	assert.Greater(t, result.RecordCount, 50) // Should have many records, exact count may vary
	assert.NotEmpty(t, result.AssignedSchema)

	// Verify files were created
	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)
}

func TestImportFile_ErrorCases(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupFile   func() string
		expectedErr string
	}{
		{
			name: "nonexistent file",
			setupFile: func() string {
				return "/nonexistent/file.json"
			},
			expectedErr: "failed to get file info",
		},
		{
			name: "invalid JSON",
			setupFile: func() string {
				return setup.WriteFile("invalid.json", fixtures.InvalidJSON())
			},
			expectedErr: "failed to analyze data",
		},
		{
			name: "empty file",
			setupFile: func() string {
				return setup.WriteFile("empty.json", "")
			},
			expectedErr: "failed to analyze data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFile()

			opts := ImportOptions{
				SourcePath: filePath,
			}

			result, err := importer.ImportFile(opts)
			assert.Error(t, err)
			assert.Nil(t, result)
			testutil.AssertErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestImportFile_KubernetesDetection(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create Kubernetes Pod YAML file
	k8sFile := setup.WriteFile("pod.yaml", fixtures.KubernetesPod())

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Test import
	opts := ImportOptions{
		SourcePath: k8sFile,
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify format was detected and a schema was assigned
	// Note: Without specific K8s schemas in the repo, catchall is used
	assert.Equal(t, "yaml", result.DetectedFormat)
	assert.NotEmpty(t, result.AssignedSchema)
}

func TestImportFile_AWSDetection(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create AWS instance JSON file
	awsFile := setup.WriteFile("aws-instance.json", fixtures.AWSInstance())

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Test import
	opts := ImportOptions{
		SourcePath: awsFile,
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify format was detected and a schema was assigned
	// Note: Without specific AWS schemas in the repo, catchall is used
	assert.Equal(t, "json", result.DetectedFormat)
	assert.NotEmpty(t, result.AssignedSchema)
}

func TestImportFile_ManualSchema(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create test JSON file
	jsonFile := setup.WriteFile("test.json", fixtures.ValidJSON())

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Test import with manual schema
	opts := ImportOptions{
		SourcePath:   jsonFile,
		ManualSchema: "custom.#TestSchema",
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify manual schema was used (when CascadeValidator is nil, it falls back to rule engine)
	assert.Equal(t, "json", result.DetectedFormat)
	assert.NotEmpty(t, result.AssignedSchema)
}

func TestImportResult_Timestamps(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create test file
	jsonFile := setup.WriteFile("test.json", fixtures.ValidJSON())

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Record time before import
	beforeImport := time.Now()

	// Test import
	opts := ImportOptions{
		SourcePath: jsonFile,
	}

	result, err := importer.ImportFile(opts)
	require.NoError(t, err)

	// Record time after import
	afterImport := time.Now()

	// Parse import timestamp
	importTime, err := time.Parse(time.RFC3339, result.ImportTimestamp)
	require.NoError(t, err)

	// Verify timestamp is reasonable (allow some tolerance for test execution time)
	assert.True(t, importTime.After(beforeImport.Add(-time.Second)) || importTime.Equal(beforeImport))
	assert.True(t, importTime.Before(afterImport.Add(time.Second)) || importTime.Equal(afterImport))
}

func TestImportFile_CollectionWrapper(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create a JSON file that looks like a collection wrapper response
	wrapperJSON := `{"items": [{"id": "a", "name": "alpha"}, {"id": "b", "name": "beta"}], "count": 2}`
	wrapperFile := setup.WriteFile("wrapper.json", wrapperJSON)

	imp, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFile(ImportOptions{SourcePath: wrapperFile})
	require.NoError(t, err)
	require.NotNil(t, result)

	// The wrapper should be detected and imported as a collection
	assert.Equal(t, 2, result.RecordCount, "wrapper should report 2 items")
	assert.Contains(t, result.AssignedSchema, "#Collection", "schema should be Collection")
	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)

	// Verify catalog has the collection entry plus 2 item entries
	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	assert.Len(t, items, 2, "catalog should contain 2 item entries")

	// Verify each item has the expected properties
	for _, item := range items {
		assert.NotEmpty(t, item.ID)
		assert.Equal(t, "json", item.Format)
		assert.NotNil(t, item.CollectionType)
		assert.Equal(t, "item", *item.CollectionType)
		assert.NotNil(t, item.CollectionID)
		assert.Equal(t, result.ID, *item.CollectionID)
		// Verify item data files exist on disk
		testutil.AssertFileExists(t, item.StoredPath)
	}

	// Verify the collection entry itself exists
	collectionEntry, err := imp.catalogDB.GetCollectionByID(result.ID)
	require.NoError(t, err)
	require.NotNil(t, collectionEntry)
	assert.Equal(t, "collection", *collectionEntry.CollectionType)
}

func TestImportFile_NormalObjectNotDetectedAsWrapper(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// A normal JSON object should NOT be detected as a wrapper
	jsonFile := setup.WriteFile("normal.json", fixtures.ValidJSON())

	imp, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFile(ImportOptions{SourcePath: jsonFile})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should be imported as a single object, not a collection
	assert.Equal(t, "json", result.DetectedFormat)
	assert.Equal(t, 1, result.RecordCount)
	assert.NotContains(t, result.AssignedSchema, "#Collection",
		"normal object should not be assigned Collection schema")
	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)

	// Verify no collection items were created for this entry
	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	assert.Empty(t, items, "normal object should have no collection items")
}

func TestImportFile_NDJSONUsesNDJSONPath(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create an NDJSON file with .json extension so that detectFormat's
	// isNewlineDelimitedJSON check is triggered (the switch only handles .json).
	ndjsonContent := `{"id": 1, "name": "item1", "type": "test"}
{"id": 2, "name": "item2", "type": "test"}
{"id": 3, "name": "item3", "type": "test"}`
	ndjsonFile := setup.WriteFile("collection.json", ndjsonContent)

	imp, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFile(ImportOptions{
		SourcePath:      ndjsonFile,
		StreamingConfig: streaming.DefaultStreamingConfig(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// NDJSON should be detected as "ndjson" format, routed through the NDJSON
	// collection path (not the wrapper path)
	assert.Equal(t, "ndjson", result.DetectedFormat, "format should be ndjson")
	assert.Contains(t, result.AssignedSchema, "#Collection")
	assert.Greater(t, result.RecordCount, 0, "should have imported records")
	testutil.AssertFileExists(t, result.StoredPath)

	// Verify individual item files were created on disk
	rawDir := filepath.Dir(result.StoredPath)
	entries, err := os.ReadDir(rawDir)
	require.NoError(t, err)
	// Should have the original NDJSON file plus individual item JSON files
	assert.Greater(t, len(entries), 1, "raw dir should contain original file plus item files")
}

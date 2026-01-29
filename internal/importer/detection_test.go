package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/test/testutil"
)

func TestDetectFormat(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name           string
		filename       string
		content        string
		expectedFormat string
	}{
		{
			name:           "JSON file by extension",
			filename:       "test.json",
			content:        fixtures.ValidJSON(),
			expectedFormat: "json",
		},
		{
			name:           "YAML file by extension",
			filename:       "test.yaml",
			content:        fixtures.ValidYAML(),
			expectedFormat: "yaml",
		},
		{
			name:           "YML file by extension",
			filename:       "test.yml",
			content:        fixtures.ValidYAML(),
			expectedFormat: "yaml",
		},
		{
			name:           "CSV file by extension",
			filename:       "test.csv",
			content:        fixtures.CSVData(),
			expectedFormat: "csv",
		},
		{
			name:           "NDJSON file",
			filename:       "test.json",
			content:        fixtures.ValidNDJSON(),
			expectedFormat: "ndjson",
		},
		{
			name:           "JSON by content detection",
			filename:       "test.txt",
			content:        fixtures.ValidJSON(),
			expectedFormat: "json",
		},
		{
			name:           "YAML by content detection",
			filename:       "test.txt",
			content:        fixtures.ValidYAML(),
			expectedFormat: "yaml",
		},
		{
			name:           "CSV by content detection",
			filename:       "test.txt",
			content:        fixtures.CSVData(),
			expectedFormat: "csv",
		},
		{
			name:           "unknown format",
			filename:       "test.bin",
			content:        "binary data \x00\x01\x02",
			expectedFormat: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := setup.WriteFile(tt.filename, tt.content)

			// Test format detection
			format, err := importer.detectFormat(filePath)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedFormat, format)
		})
	}
}

func TestDetectFormat_ErrorCases(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name        string
		filePath    string
		expectedErr string
		expectError bool
	}{
		{
			name:        "nonexistent file",
			filePath:    "/nonexistent/file.json",
			expectedErr: "no such file or directory",
			expectError: false, // detectFormat returns "json" for .json extension even if file doesn't exist
		},
		{
			name:        "directory instead of file",
			filePath:    setup.TempDir(),
			expectedErr: "is a directory",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, err := importer.detectFormat(tt.filePath)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, format)
				testutil.AssertErrorContains(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, format)
			}
		})
	}
}

func TestIsNewlineDelimitedJSON(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name      string
		content   string
		isNDJSON  bool
		shouldErr bool
	}{
		{
			name:      "valid NDJSON",
			content:   fixtures.ValidNDJSON(),
			isNDJSON:  true,
			shouldErr: false,
		},
		{
			name:      "regular JSON object",
			content:   fixtures.ValidJSON(),
			isNDJSON:  false,
			shouldErr: false,
		},
		{
			name:      "regular JSON array",
			content:   fixtures.ValidJSONArray(),
			isNDJSON:  false,
			shouldErr: false,
		},
		{
			name:      "single line JSON",
			content:   `{"id": 1, "name": "test"}`,
			isNDJSON:  false,
			shouldErr: false,
		},
		{
			name:      "empty file",
			content:   "",
			isNDJSON:  false,
			shouldErr: false,
		},
		{
			name:      "invalid JSON lines",
			content:   "not json\nstill not json",
			isNDJSON:  false,
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := setup.WriteFile("test.json", tt.content)

			// Test NDJSON detection
			isNDJSON, err := importer.isNewlineDelimitedJSON(filePath)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.isNDJSON, isNDJSON)
			}
		})
	}
}

func TestDetectOrigin(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// detectOrigin now simply returns the filename without extension.
	// Schema detection should be handled by CUE-based inference, not hardcoded patterns.
	tests := []struct {
		name           string
		filename       string
		format         string
		expectedOrigin string
	}{
		{
			name:           "AWS EC2 instances file",
			filename:       "aws-ec2-instances.json",
			format:         "json",
			expectedOrigin: "aws-ec2-instances",
		},
		{
			name:           "AWS S3 buckets file",
			filename:       "aws-s3-buckets.json",
			format:         "json",
			expectedOrigin: "aws-s3-buckets",
		},
		{
			name:           "Kubernetes pods file",
			filename:       "k8s-pods.yaml",
			format:         "yaml",
			expectedOrigin: "k8s-pods",
		},
		{
			name:           "Docker containers file",
			filename:       "docker-containers.json",
			format:         "json",
			expectedOrigin: "docker-containers",
		},
		{
			name:           "generic data file",
			filename:       "data.json",
			format:         "json",
			expectedOrigin: "data",
		},
		{
			name:           "uppercase filename normalized to lowercase",
			filename:       "MyData.JSON",
			format:         "json",
			expectedOrigin: "mydata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := setup.WriteFile(tt.filename, "{}")

			// Test origin detection
			origin := importer.detectOrigin(filePath, tt.format)
			assert.Equal(t, tt.expectedOrigin, origin)
		})
	}
}

func TestAnalyzeData_JSON(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name          string
		content       string
		expectedCount int
		expectError   bool
		validateData  func(t *testing.T, data interface{})
	}{
		{
			name:          "valid JSON object",
			content:       fixtures.ValidJSON(),
			expectedCount: 1,
			expectError:   false,
			validateData: func(t *testing.T, data interface{}) {
				dataMap, ok := data.(map[string]interface{})
				require.True(t, ok, "Data should be a map")
				assert.Equal(t, "test-item", dataMap["name"])
				assert.Equal(t, "example", dataMap["type"])
				assert.Equal(t, float64(42), dataMap["count"])
			},
		},
		{
			name:          "valid JSON array",
			content:       fixtures.ValidJSONArray(),
			expectedCount: 2,
			expectError:   false,
			validateData: func(t *testing.T, data interface{}) {
				dataArray, ok := data.([]interface{})
				require.True(t, ok, "Data should be an array")
				assert.Len(t, dataArray, 2)
			},
		},
		{
			name:        "invalid JSON",
			content:     fixtures.InvalidJSON(),
			expectError: true,
		},
		{
			name:        "empty file",
			content:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := setup.WriteFile("test.json", tt.content)

			// Test data analysis using streaming
			data, count, err := importer.analyzeDataStreaming(filePath, "json", nil)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedCount, count)
				if tt.validateData != nil {
					tt.validateData(t, data)
				}
			}
		})
	}
}

func TestAnalyzeData_YAML(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name          string
		content       string
		expectedCount int
		expectError   bool
	}{
		{
			name:          "valid YAML",
			content:       fixtures.ValidYAML(),
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:        "invalid YAML",
			content:     fixtures.InvalidYAML(),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := setup.WriteFile("test.yaml", tt.content)

			// Test data analysis using streaming
			data, count, err := importer.analyzeDataStreaming(filePath, "yaml", nil)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedCount, count)
				assert.NotNil(t, data)
			}
		})
	}
}

func TestAnalyzeData_CSV(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Create test file
	filePath := setup.WriteFile("test.csv", fixtures.CSVData())

	// Test data analysis using streaming
	data, count, err := importer.analyzeDataStreaming(filePath, "csv", nil)
	require.NoError(t, err)

	// CSV should return 3 records (excluding header)
	assert.Equal(t, 3, count)
	assert.NotNil(t, data)

	// Verify data structure - CSV returns array of maps
	dataArray, ok := data.([]map[string]string)
	require.True(t, ok, "CSV data should be an array of maps")
	assert.Len(t, dataArray, 3)
}

func TestAnalyzeData_UnknownFormat(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	// Create test file with unknown format
	filePath := setup.WriteFile("test.bin", "binary data")

	// Test data analysis using streaming
	data, count, err := importer.analyzeDataStreaming(filePath, "unknown", nil)
	require.NoError(t, err)

	// Unknown format should return basic info
	assert.Equal(t, 1, count)
	assert.NotNil(t, data)

	dataMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Unknown format data should be a map")
	assert.Equal(t, "unknown", dataMap["format"])
}

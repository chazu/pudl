package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/streaming"
	"github.com/chazu/pudl/test/testutil"
)

// These tests exercise the live import pipeline — EnhancedImporter.ImportFileWithFriendlyIDs,
// the path cmd/import.go actually uses. They were ported from the now-removed base
// Importer.ImportFile tests when that legacy path was deleted; collection-*wrapper*
// detection had no live equivalent and was dropped with it.

func TestNewEnhancedImporter(t *testing.T) {
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
			imp, err := NewEnhancedImporter(tt.dataPath, tt.schemaPath, tt.pudlHome)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, imp)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, imp)
				assert.Equal(t, tt.dataPath, imp.dataPath)
				assert.Equal(t, tt.schemaPath, imp.schemaPath)
				assert.NotNil(t, imp.catalogDB)
				assert.NotNil(t, imp.inferrer)
			}
		})
	}
}

func TestImport_JSON(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	jsonFile := setup.WriteFile("test.json", fixtures.ValidJSON())

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	opts := ImportOptions{
		SourcePath:   jsonFile,
		UseStreaming: false,
	}

	result, err := imp.ImportFileWithFriendlyIDs(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, jsonFile, result.SourcePath)
	assert.Equal(t, "json", result.DetectedFormat)
	assert.NotEmpty(t, result.StoredPath)
	assert.NotEmpty(t, result.MetadataPath)
	assert.NotEmpty(t, result.AssignedSchema)
	assert.Greater(t, result.SchemaConfidence, 0.0)
	assert.Equal(t, 1, result.RecordCount)
	assert.Greater(t, result.SizeBytes, int64(0))
	assert.NotEmpty(t, result.ImportTimestamp)
	assert.NotEmpty(t, result.ContentHash)

	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)
	testutil.AssertFileContains(t, result.MetadataPath, result.ID)
	testutil.AssertFileContains(t, result.MetadataPath, result.AssignedSchema)
}

func TestImport_YAML(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	yamlFile := setup.WriteFile("test.yaml", fixtures.ValidYAML())

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: yamlFile})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "yaml", result.DetectedFormat)
	assert.NotEmpty(t, result.AssignedSchema)
	assert.Equal(t, 1, result.RecordCount)

	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)
}

func TestImport_WithStreaming(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	largeFile := setup.WriteFile("large.json", fixtures.LargeNDJSON(100))

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	opts := ImportOptions{
		SourcePath:   largeFile,
		UseStreaming: true,
		StreamingConfig: &streaming.StreamingConfig{
			ChunkAlgorithm: "fastcdc",
			MinChunkSize:   512,
			MaxChunkSize:   2048,
			AvgChunkSize:   1024,
			BufferSize:     4096,
			MaxMemoryMB:    10,
			ErrorTolerance: 0.1,
			SkipMalformed:  true,
			SampleSize:     100,
			Confidence:     0.8,
			ReportEveryMB:  1,
			MaxConcurrency: 0,
		},
	}

	result, err := imp.ImportFileWithFriendlyIDs(opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "ndjson", result.DetectedFormat)
	assert.Greater(t, result.RecordCount, 50) // many records; exact count may vary
	assert.NotEmpty(t, result.AssignedSchema)

	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)
}

func TestImport_ErrorCases(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupFile   func() string
		expectedErr string
	}{
		{
			name:        "nonexistent file",
			setupFile:   func() string { return "/nonexistent/file.json" },
			expectedErr: "failed to get file info",
		},
		{
			name:        "invalid JSON",
			setupFile:   func() string { return setup.WriteFile("invalid.json", fixtures.InvalidJSON()) },
			expectedErr: "failed to analyze data",
		},
		{
			name:        "empty file",
			setupFile:   func() string { return setup.WriteFile("empty.json", "") },
			expectedErr: "failed to analyze data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: tt.setupFile()})
			assert.Error(t, err)
			assert.Nil(t, result)
			testutil.AssertErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestImport_KubernetesDetection(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	k8sFile := setup.WriteFile("pod.yaml", fixtures.KubernetesPod())

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: k8sFile})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Without specific K8s schemas in the repo, catchall is used.
	assert.Equal(t, "yaml", result.DetectedFormat)
	assert.NotEmpty(t, result.AssignedSchema)
}

func TestImport_AWSDetection(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	awsFile := setup.WriteFile("aws-instance.json", fixtures.AWSInstance())

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: awsFile})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Without specific AWS schemas in the repo, catchall is used.
	assert.Equal(t, "json", result.DetectedFormat)
	assert.NotEmpty(t, result.AssignedSchema)
}

func TestImportResult_Timestamps(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	jsonFile := setup.WriteFile("test.json", fixtures.ValidJSON())

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	beforeImport := time.Now()
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: jsonFile})
	require.NoError(t, err)
	afterImport := time.Now()

	importTime, err := time.Parse(time.RFC3339, result.ImportTimestamp)
	require.NoError(t, err)

	assert.True(t, importTime.After(beforeImport.Add(-time.Second)) || importTime.Equal(beforeImport))
	assert.True(t, importTime.Before(afterImport.Add(time.Second)) || importTime.Equal(afterImport))
}

func TestImport_NormalObjectIsSingleEntry(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	jsonFile := setup.WriteFile("normal.json", fixtures.ValidJSON())

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: jsonFile})
	require.NoError(t, err)
	require.NotNil(t, result)

	// A plain JSON object imports as a single entry, never a collection.
	assert.Equal(t, "json", result.DetectedFormat)
	assert.Equal(t, 1, result.RecordCount)
	assert.NotContains(t, result.AssignedSchema, "#Collection",
		"normal object should not be assigned Collection schema")
	testutil.AssertFileExists(t, result.StoredPath)
	testutil.AssertFileExists(t, result.MetadataPath)

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	assert.Empty(t, items, "normal object should have no collection items")
}

// TestImportNDJSON_LinuxSchemaRouting imports NDJSON whose records carry _schema
// fields for pudl/linux schemas. Expected:
//  1. A Collection entry is created for the NDJSON file as a whole.
//  2. Each line becomes an individual Item entry in the catalog.
//  3. Each item is routed to the correct pudl/linux.#* schema (not catchall #Item).
func TestImportNDJSON_LinuxSchemaRouting(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	setup.AddBootstrapSchemas(workspace)

	ndjsonContent := `{"_schema":"linux.host","hostname":"renge","os":{"id":"debian","version":"10","name":"Debian GNU/Linux 10 (buster)"},"kernel":"5.10.0-odroid-arm64","arch":"aarch64","uptime_seconds":12114}
{"_schema":"linux.package","host":"renge","name":"acl","version":"2.2.53-4","status":"ii "}
{"_schema":"linux.package","host":"renge","name":"adduser","version":"3.118","status":"ii "}
{"_schema":"linux.service","host":"renge","unit":"ssh.service","active":"active","sub":"running"}
{"_schema":"linux.service","host":"renge","unit":"cron.service","active":"active","sub":"running"}
{"_schema":"linux.filesystem","host":"renge","device":"/dev/mmcblk1p2","mountpoint":"/","fstype":"ext4","size_bytes":15267692544,"used_bytes":3841851392,"avail_bytes":10626351104}
{"_schema":"linux.user","host":"renge","name":"root","uid":0,"gid":0,"home":"/root","shell":"/bin/bash"}
{"_schema":"linux.user","host":"renge","name":"nobody","uid":65534,"gid":65534,"home":"/nonexistent","shell":"/usr/sbin/nologin"}`

	ndjsonFile := setup.WriteFile("observe.ndjson", ndjsonContent)

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{
		SourcePath:      ndjsonFile,
		StreamingConfig: streaming.DefaultStreamingConfig(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "ndjson", result.DetectedFormat, "format should be ndjson")
	assert.Contains(t, result.AssignedSchema, "#Collection", "top-level schema should be Collection")
	assert.Equal(t, 8, result.RecordCount, "should have 8 records")

	collectionEntry, err := imp.catalogDB.GetCollectionByID(result.ID)
	require.NoError(t, err)
	require.NotNil(t, collectionEntry, "collection entry should exist")
	assert.Equal(t, "collection", *collectionEntry.CollectionType)

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	assert.Len(t, items, 8, "should have 8 item entries")

	schemaCount := map[string]int{}
	for _, item := range items {
		schemaCount[item.Schema]++
		require.NotNil(t, item.CollectionID)
		assert.Equal(t, result.ID, *item.CollectionID)
		require.NotNil(t, item.CollectionType)
		assert.Equal(t, "item", *item.CollectionType)
		assert.NotContains(t, item.Schema, "#Item",
			"item %s should not be assigned catchall #Item schema, got %s", item.ID, item.Schema)
	}

	// Expected distribution: 1 Host, 2 Package, 2 Service, 1 Filesystem, 2 User.
	hostCount, pkgCount, svcCount, fsCount, userCount := 0, 0, 0, 0, 0
	for schema, count := range schemaCount {
		switch {
		case strings.Contains(schema, "#Host"):
			hostCount += count
		case strings.Contains(schema, "#Package"):
			pkgCount += count
		case strings.Contains(schema, "#Service"):
			svcCount += count
		case strings.Contains(schema, "#Filesystem"):
			fsCount += count
		case strings.Contains(schema, "#User"):
			userCount += count
		}
	}
	assert.Equal(t, 1, hostCount, "expected 1 Host record")
	assert.Equal(t, 2, pkgCount, "expected 2 Package records")
	assert.Equal(t, 2, svcCount, "expected 2 Service records")
	assert.Equal(t, 1, fsCount, "expected 1 Filesystem record")
	assert.Equal(t, 2, userCount, "expected 2 User records")
}

func TestImport_NDJSONUsesNDJSONPath(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// .json extension with newline-delimited content triggers detectFormat's
	// isNewlineDelimitedJSON check (the switch only handles .json).
	ndjsonContent := `{"id": 1, "name": "item1", "type": "test"}
{"id": 2, "name": "item2", "type": "test"}
{"id": 3, "name": "item3", "type": "test"}`
	ndjsonFile := setup.WriteFile("collection.json", ndjsonContent)

	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{
		SourcePath:      ndjsonFile,
		StreamingConfig: streaming.DefaultStreamingConfig(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "ndjson", result.DetectedFormat, "format should be ndjson")
	assert.Contains(t, result.AssignedSchema, "#Collection")
	assert.Greater(t, result.RecordCount, 0, "should have imported records")
	testutil.AssertFileExists(t, result.StoredPath)

	rawDir := filepath.Dir(result.StoredPath)
	entries, err := os.ReadDir(rawDir)
	require.NoError(t, err)
	assert.Greater(t, len(entries), 1, "raw dir should contain original file plus item files")
}

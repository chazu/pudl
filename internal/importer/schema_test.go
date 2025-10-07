package importer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pudl/test/testutil"
)

func TestAssignSchema(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name               string
		data               interface{}
		origin             string
		format             string
		expectedSchema     string
		minConfidence      float64
		maxConfidence      float64
	}{
		{
			name: "Kubernetes Pod",
			data: map[string]interface{}{
				"kind":       "Pod",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test-pod",
				},
			},
			origin:         "k8s-pods",
			format:         "yaml",
			expectedSchema: "k8s.#Pod",
			minConfidence:  0.9,
			maxConfidence:  1.0,
		},
		{
			name: "Kubernetes Service",
			data: map[string]interface{}{
				"kind":       "Service",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test-service",
				},
			},
			origin:         "k8s-services",
			format:         "yaml",
			expectedSchema: "k8s.#Service",
			minConfidence:  0.85,
			maxConfidence:  1.0,
		},
		{
			name: "Generic Kubernetes Resource",
			data: map[string]interface{}{
				"kind":       "CustomResource",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test-resource",
				},
			},
			origin:         "k8s-unknown",
			format:         "yaml",
			expectedSchema: "k8s.#CustomResource",
			minConfidence:  0.8,
			maxConfidence:  1.0,
		},
		{
			name: "AWS API Response",
			data: map[string]interface{}{
				"ResponseMetadata": map[string]interface{}{
					"RequestId": "12345",
				},
				"Instances": []interface{}{},
			},
			origin:         "aws-ec2",
			format:         "json",
			expectedSchema: "aws.#APIResponse",
			minConfidence:  0.7,
			maxConfidence:  1.0,
		},
		{
			name: "AWS EC2 Instance",
			data: map[string]interface{}{
				"InstanceId": "i-1234567890abcdef0",
				"ImageId":    "ami-12345678",
				"State": map[string]interface{}{
					"Name": "running",
				},
			},
			origin:         "aws-ec2-instances",
			format:         "json",
			expectedSchema: "aws.#EC2Resource",
			minConfidence:  0.5,
			maxConfidence:  1.0,
		},
		{
			name: "AWS S3 Resource",
			data: map[string]interface{}{
				"BucketName": "my-bucket",
				"Region":     "us-east-1",
			},
			origin:         "aws-s3-buckets",
			format:         "json",
			expectedSchema: "aws.#S3Resource",
			minConfidence:  0.5,
			maxConfidence:  1.0,
		},
		{
			name: "Generic AWS Resource",
			data: map[string]interface{}{
				"ResourceId": "resource-123",
				"Tags":       []interface{}{},
			},
			origin:         "aws-unknown",
			format:         "json",
			expectedSchema: "aws.#Resource",
			minConfidence:  0.4,
			maxConfidence:  0.6,
		},
		{
			name: "Unknown Data",
			data: map[string]interface{}{
				"someField": "someValue",
				"count":     42,
			},
			origin:         "unknown",
			format:         "json",
			expectedSchema: "unknown.#CatchAll",
			minConfidence:  0.0,
			maxConfidence:  0.2,
		},
		{
			name:           "Array Data",
			data:           []interface{}{map[string]interface{}{"id": 1}, map[string]interface{}{"id": 2}},
			origin:         "unknown",
			format:         "json",
			expectedSchema: "unknown.#CatchAll",
			minConfidence:  0.0,
			maxConfidence:  0.2,
		},
		{
			name:           "Non-map Data",
			data:           "simple string",
			origin:         "unknown",
			format:         "json",
			expectedSchema: "unknown.#CatchAll",
			minConfidence:  0.0,
			maxConfidence:  0.2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, confidence := importer.assignSchema(tt.data, tt.origin, tt.format)

			assert.Equal(t, tt.expectedSchema, schema)
			assert.GreaterOrEqual(t, confidence, tt.minConfidence)
			assert.LessOrEqual(t, confidence, tt.maxConfidence)
		})
	}
}

func TestHasFields(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name     string
		data     map[string]interface{}
		fields   []string
		expected bool
	}{
		{
			name: "all fields present",
			data: map[string]interface{}{
				"field1": "value1",
				"field2": "value2",
				"field3": "value3",
			},
			fields:   []string{"field1", "field2"},
			expected: true,
		},
		{
			name: "some fields missing",
			data: map[string]interface{}{
				"field1": "value1",
				"field3": "value3",
			},
			fields:   []string{"field1", "field2"},
			expected: false,
		},
		{
			name: "no fields required",
			data: map[string]interface{}{
				"field1": "value1",
			},
			fields:   []string{},
			expected: true,
		},
		{
			name:     "empty data",
			data:     map[string]interface{}{},
			fields:   []string{"field1"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := importer.hasFields(tt.data, tt.fields)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAddToCatalog is removed since addToCatalog is not a public method
// The functionality is tested through ImportFile tests instead

func TestSchemaDefinitions(t *testing.T) {
	// Test that schema definitions are properly defined
	tests := []struct {
		name           string
		schemaKey      string
		expectedPkg    string
		expectedDef    string
		hasIdentity    bool
		hasTracked     bool
	}{
		{
			name:        "AWS EC2 Instance",
			schemaKey:   "aws.#EC2Instance",
			expectedPkg: "aws",
			expectedDef: "#EC2Instance",
			hasIdentity: true,
			hasTracked:  true,
		},
		{
			name:        "Kubernetes Pod",
			schemaKey:   "k8s.#Pod",
			expectedPkg: "k8s",
			expectedDef: "#Pod",
			hasIdentity: true,
			hasTracked:  true,
		},
		{
			name:        "Unknown CatchAll",
			schemaKey:   "unknown.#CatchAll",
			expectedPkg: "unknown",
			expectedDef: "#CatchAll",
			hasIdentity: false,
			hasTracked:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaDef, exists := basicSchemaDefinitions[tt.schemaKey]
			require.True(t, exists, "Schema definition should exist for %s", tt.schemaKey)

			assert.Equal(t, tt.expectedPkg, schemaDef.Package)
			assert.Equal(t, tt.expectedDef, schemaDef.Definition)
			assert.Equal(t, "v1.0", schemaDef.Version)

			if tt.hasIdentity {
				assert.NotEmpty(t, schemaDef.IdentityFields, "Should have identity fields")
			} else {
				assert.Empty(t, schemaDef.IdentityFields, "Should not have identity fields")
			}

			if tt.hasTracked {
				assert.NotEmpty(t, schemaDef.TrackedFields, "Should have tracked fields")
			} else {
				assert.Empty(t, schemaDef.TrackedFields, "Should not have tracked fields")
			}
		})
	}
}

func TestExtractPackage(t *testing.T) {
	tests := []struct {
		name           string
		schema         string
		expectedPkg    string
	}{
		{
			name:        "standard schema",
			schema:      "aws.#EC2Instance",
			expectedPkg: "aws",
		},
		{
			name:        "nested schema",
			schema:      "k8s.apps.#Deployment",
			expectedPkg: "k8s",
		},
		{
			name:        "no package",
			schema:      "#SimpleSchema",
			expectedPkg: "unknown",
		},
		{
			name:        "empty schema",
			schema:      "",
			expectedPkg: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := extractPackage(tt.schema)
			assert.Equal(t, tt.expectedPkg, pkg)
		})
	}
}

func TestAssignSchema_EdgeCases(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name          string
		data          interface{}
		origin        string
		format        string
		expectedSchema string
	}{
		{
			name: "Kubernetes resource without kind",
			data: map[string]interface{}{
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test",
				},
			},
			origin:         "k8s-unknown",
			format:         "yaml",
			expectedSchema: "k8s.#Resource",
		},
		{
			name: "AWS resource with partial fields",
			data: map[string]interface{}{
				"InstanceId": "i-123",
				// Missing other typical EC2 fields
			},
			origin:         "aws-ec2",
			format:         "json",
			expectedSchema: "aws.#EC2Resource",
		},
		{
			name: "Array with mixed types",
			data: []interface{}{
				map[string]interface{}{"type": "user"},
				"string item",
				42,
			},
			origin:         "unknown",
			format:         "json",
			expectedSchema: "unknown.#CatchAll",
		},
		{
			name: "Empty map",
			data: map[string]interface{}{},
			origin:         "unknown",
			format:         "json",
			expectedSchema: "unknown.#CatchAll",
		},
		{
			name:           "Nil data",
			data:           nil,
			origin:         "unknown",
			format:         "json",
			expectedSchema: "unknown.#CatchAll",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, confidence := importer.assignSchema(tt.data, tt.origin, tt.format)
			assert.Equal(t, tt.expectedSchema, schema)
			assert.GreaterOrEqual(t, confidence, 0.0)
			assert.LessOrEqual(t, confidence, 1.0)
		})
	}
}

func TestAssignSchema_RealWorldData(t *testing.T) {
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	fixtures := testutil.NewTestDataFixtures()

	// Create importer
	importer, err := New(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)

	tests := []struct {
		name           string
		jsonData       string
		origin         string
		expectedSchema string
		minConfidence  float64
	}{
		{
			name:           "Kubernetes Pod JSON",
			jsonData:       `{"apiVersion": "v1", "kind": "Pod", "metadata": {"name": "test-pod", "namespace": "default"}}`,
			origin:         "k8s-pods",
			expectedSchema: "k8s.#Pod",
			minConfidence:  0.9,
		},
		{
			name:           "AWS EC2 Instance",
			jsonData:       fixtures.AWSInstance(),
			origin:         "aws-ec2-instances",
			expectedSchema: "aws.#EC2Instance",
			minConfidence:  0.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse JSON data
			var data interface{}
			err := json.Unmarshal([]byte(tt.jsonData), &data)
			require.NoError(t, err)

			// Test schema assignment
			schema, confidence := importer.assignSchema(data, tt.origin, "json")
			assert.Equal(t, tt.expectedSchema, schema)
			assert.GreaterOrEqual(t, confidence, tt.minConfidence)
		})
	}
}

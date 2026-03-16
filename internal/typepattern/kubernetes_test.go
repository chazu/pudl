package typepattern

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractKubernetesTypeID(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected string
	}{
		{
			name: "batch/v1 Job",
			data: map[string]interface{}{
				"apiVersion": "batch/v1",
				"kind":       "Job",
			},
			expected: "batch/v1:Job",
		},
		{
			name: "apps/v1 Deployment",
			data: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
			},
			expected: "apps/v1:Deployment",
		},
		{
			name: "core v1 Pod",
			data: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
			},
			expected: "v1:Pod",
		},
		{
			name: "missing apiVersion",
			data: map[string]interface{}{
				"kind": "Pod",
			},
			expected: "",
		},
		{
			name: "missing kind",
			data: map[string]interface{}{
				"apiVersion": "v1",
			},
			expected: "",
		},
		{
			name:     "empty data",
			data:     map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKubernetesTypeID(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapKubernetesImport(t *testing.T) {
	tests := []struct {
		typeID   string
		expected string
	}{
		{"batch/v1:Job", "cue.dev/x/k8s.io/api/batch/v1:Job"},
		{"batch/v1:CronJob", "cue.dev/x/k8s.io/api/batch/v1:CronJob"},
		{"apps/v1:Deployment", "cue.dev/x/k8s.io/api/apps/v1:Deployment"},
		{"apps/v1:StatefulSet", "cue.dev/x/k8s.io/api/apps/v1:StatefulSet"},
		{"apps/v1:DaemonSet", "cue.dev/x/k8s.io/api/apps/v1:DaemonSet"},
		{"apps/v1:ReplicaSet", "cue.dev/x/k8s.io/api/apps/v1:ReplicaSet"},
		{"v1:Pod", "cue.dev/x/k8s.io/api/core/v1:Pod"},
		{"v1:Service", "cue.dev/x/k8s.io/api/core/v1:Service"},
		{"v1:ConfigMap", "cue.dev/x/k8s.io/api/core/v1:ConfigMap"},
		{"v1:Secret", "cue.dev/x/k8s.io/api/core/v1:Secret"},
		{"v1:ServiceAccount", "cue.dev/x/k8s.io/api/core/v1:ServiceAccount"},
		{"v1:PersistentVolumeClaim", "cue.dev/x/k8s.io/api/core/v1:PersistentVolumeClaim"},
		{"networking.k8s.io/v1:Ingress", "cue.dev/x/k8s.io/api/networking/v1:Ingress"},
		{"networking.k8s.io/v1:NetworkPolicy", "cue.dev/x/k8s.io/api/networking/v1:NetworkPolicy"},
		{"rbac.authorization.k8s.io/v1:Role", "cue.dev/x/k8s.io/api/rbac/v1:Role"},
		{"rbac.authorization.k8s.io/v1:ClusterRole", "cue.dev/x/k8s.io/api/rbac/v1:ClusterRole"},
		// Unknown type returns empty string
		{"custom.example.com/v1:MyResource", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.typeID, func(t *testing.T) {
			result := mapKubernetesImport(tt.typeID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractAPIGroup(t *testing.T) {
	tests := []struct {
		typeID   string
		expected string
	}{
		{"batch/v1:Job", "batch"},
		{"apps/v1:Deployment", "apps"},
		{"v1:Pod", "core"},
		{"networking.k8s.io/v1:Ingress", "networking"},
		{"rbac.authorization.k8s.io/v1:Role", "rbac"},
		{"", "core"},
		{"NoColon", "core"},
	}

	for _, tt := range tests {
		t.Run(tt.typeID, func(t *testing.T) {
			result := extractAPIGroup(tt.typeID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKubernetesMetadataDefaults(t *testing.T) {
	tests := []struct {
		typeID             string
		expectedSchemaType string
		expectedResType    string
		expectedTracked    []string
	}{
		{
			typeID:             "batch/v1:Job",
			expectedSchemaType: "kubernetes",
			expectedResType:    "k8s.batch.job",
			expectedTracked:    []string{"status.succeeded", "status.failed", "status.active"},
		},
		{
			typeID:             "v1:Pod",
			expectedSchemaType: "kubernetes",
			expectedResType:    "k8s.core.pod",
			expectedTracked:    []string{"status.phase", "status.conditions"},
		},
		{
			typeID:             "apps/v1:Deployment",
			expectedSchemaType: "kubernetes",
			expectedResType:    "k8s.apps.deployment",
			expectedTracked:    []string{"status.replicas", "status.readyReplicas", "status.availableReplicas"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.typeID, func(t *testing.T) {
			meta := kubernetesMetadataDefaults(tt.typeID)
			require.NotNil(t, meta)
			assert.Equal(t, tt.expectedSchemaType, meta.SchemaType)
			assert.Equal(t, tt.expectedResType, meta.ResourceType)
			assert.Equal(t, []string{"metadata.name", "metadata.namespace"}, meta.IdentityFields)
			assert.Equal(t, tt.expectedTracked, meta.TrackedFields)
		})
	}
}

func TestKubernetesMetadataDefaults_UnknownType(t *testing.T) {
	meta := kubernetesMetadataDefaults("custom/v1:MyResource")
	require.NotNil(t, meta)
	assert.Equal(t, "kubernetes", meta.SchemaType)
	assert.Equal(t, "k8s.custom.myresource", meta.ResourceType)
	assert.Empty(t, meta.TrackedFields)
}

func TestRegisterKubernetesPatterns(t *testing.T) {
	r := NewRegistry()
	RegisterKubernetesPatterns(r)

	patterns := r.GetPatternsByEcosystem("kubernetes")
	require.Len(t, patterns, 1)

	p := patterns[0]
	assert.Equal(t, "kubernetes", p.Name)
	assert.Equal(t, "kubernetes", p.Ecosystem)
	assert.Equal(t, 100, p.Priority)
	assert.Equal(t, []string{"apiVersion", "kind", "metadata"}, p.RequiredFields)
	assert.Contains(t, p.OptionalFields, "spec")
	assert.Contains(t, p.OptionalFields, "status")
	assert.NotNil(t, p.TypeExtractor)
	assert.NotNil(t, p.ImportMapper)
	assert.NotNil(t, p.MetadataDefaults)
}

func TestKubernetesPattern_Detection(t *testing.T) {
	r := NewRegistry()
	RegisterKubernetesPatterns(r)

	tests := []struct {
		name           string
		data           map[string]interface{}
		expectMatch    bool
		expectedTypeID string
		expectedImport string
	}{
		{
			name: "valid Job",
			data: map[string]interface{}{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"metadata": map[string]interface{}{
					"name":      "my-job",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
			},
			expectMatch:    true,
			expectedTypeID: "batch/v1:Job",
			expectedImport: "cue.dev/x/k8s.io/api/batch/v1:Job",
		},
		{
			name: "valid Pod with status",
			data: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name": "my-pod",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectMatch:    true,
			expectedTypeID: "v1:Pod",
			expectedImport: "cue.dev/x/k8s.io/api/core/v1:Pod",
		},
		{
			name: "missing metadata - no match",
			data: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
			},
			expectMatch: false,
		},
		{
			name: "missing apiVersion - no match",
			data: map[string]interface{}{
				"kind": "Pod",
				"metadata": map[string]interface{}{
					"name": "my-pod",
				},
			},
			expectMatch: false,
		},
		{
			name: "unknown resource type - match but empty import",
			data: map[string]interface{}{
				"apiVersion": "custom.example.com/v1",
				"kind":       "MyCustomResource",
				"metadata": map[string]interface{}{
					"name": "my-cr",
				},
			},
			expectMatch:    true,
			expectedTypeID: "custom.example.com/v1:MyCustomResource",
			expectedImport: "", // Unknown types return empty import
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Detect(tt.data)

			if !tt.expectMatch {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, "kubernetes", result.Pattern.Name)
			assert.Equal(t, tt.expectedTypeID, result.TypeID)
			assert.Equal(t, tt.expectedImport, result.ImportPath)
			assert.Greater(t, result.Confidence, 0.0)
		})
	}
}

func TestKubernetesPattern_DetectDeployment(t *testing.T) {
	r := NewRegistry()
	RegisterKubernetesPatterns(r)

	data := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "nginx-deployment",
			"namespace": "production",
		},
		"spec": map[string]interface{}{
			"replicas": 3,
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "nginx",
				},
			},
		},
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "kubernetes", result.Pattern.Name)
	assert.Equal(t, "apps/v1:Deployment", result.TypeID)
	assert.Equal(t, "cue.dev/x/k8s.io/api/apps/v1:Deployment", result.ImportPath)
	assert.Equal(t, "Deployment", result.Definition)
	assert.Greater(t, result.Confidence, 0.5)
}

func TestKubernetesPattern_ConfidenceBoost(t *testing.T) {
	r := NewRegistry()
	RegisterKubernetesPatterns(r)

	// Known kind should have higher confidence than unknown kind
	knownKind := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": "test"},
	}
	unknownKind := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "SomeUnknownKind",
		"metadata":   map[string]interface{}{"name": "test"},
	}

	knownResult := r.Detect(knownKind)
	unknownResult := r.Detect(unknownKind)

	require.NotNil(t, knownResult)
	require.NotNil(t, unknownResult)

	assert.Greater(t, knownResult.Confidence, unknownResult.Confidence,
		"Known kind should have higher confidence than unknown kind")
}

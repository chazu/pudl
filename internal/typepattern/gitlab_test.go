package typepattern

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsGitLabJob(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{
			name: "job with script",
			value: map[string]interface{}{
				"script": []interface{}{"echo hello"},
			},
			expected: true,
		},
		{
			name: "job with extends",
			value: map[string]interface{}{
				"extends": ".base",
			},
			expected: true,
		},
		{
			name: "job with trigger",
			value: map[string]interface{}{
				"trigger": "other-project",
			},
			expected: true,
		},
		{
			name: "job with script and stage",
			value: map[string]interface{}{
				"script": []interface{}{"echo test"},
				"stage":  "build",
			},
			expected: true,
		},
		{
			name:     "string value",
			value:    "not a job",
			expected: false,
		},
		{
			name: "object without job fields",
			value: map[string]interface{}{
				"image": "alpine",
			},
			expected: false,
		},
		{
			name:     "nil value",
			value:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitLabJob(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectGitLabCI(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected bool
	}{
		{
			name: "pipeline with stages and job",
			data: map[string]interface{}{
				"stages": []interface{}{"build", "test", "deploy"},
				"build_job": map[string]interface{}{
					"stage":  "build",
					"script": []interface{}{"make build"},
				},
			},
			expected: true,
		},
		{
			name: "pipeline without explicit stages",
			data: map[string]interface{}{
				"test": map[string]interface{}{
					"script": []interface{}{"make test"},
				},
			},
			expected: true,
		},
		{
			name: "pipeline with extends",
			data: map[string]interface{}{
				".base": map[string]interface{}{
					"image": "alpine",
				},
				"test": map[string]interface{}{
					"extends": ".base",
					"script":  []interface{}{"make test"},
				},
			},
			expected: true,
		},
		{
			name: "pipeline with trigger job",
			data: map[string]interface{}{
				"trigger_downstream": map[string]interface{}{
					"trigger": "downstream/project",
				},
			},
			expected: true,
		},
		{
			name: "empty data",
			data: map[string]interface{}{},
			expected: false,
		},
		{
			name: "only stages no jobs",
			data: map[string]interface{}{
				"stages": []interface{}{"build", "test"},
			},
			expected: false,
		},
		{
			name: "only variables",
			data: map[string]interface{}{
				"variables": map[string]interface{}{
					"CI": "true",
				},
			},
			expected: false,
		},
		{
			name: "kubernetes resource - not gitlab",
			data: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name": "test",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectGitLabCI(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractGitLabCITypeID(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected string
	}{
		{
			name: "valid pipeline",
			data: map[string]interface{}{
				"test": map[string]interface{}{
					"script": []interface{}{"make test"},
				},
			},
			expected: "gitlab-ci:Pipeline",
		},
		{
			name:     "non-gitlab data",
			data:     map[string]interface{}{"foo": "bar"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractGitLabCITypeID(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapGitLabCIImport(t *testing.T) {
	tests := []struct {
		typeID   string
		expected string
	}{
		{"gitlab-ci:Pipeline", "cue.dev/x/gitlab/gitlabci:Pipeline"},
		{"other:Type", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.typeID, func(t *testing.T) {
			result := mapGitLabCIImport(tt.typeID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitLabCIMetadataDefaults(t *testing.T) {
	meta := gitlabCIMetadataDefaults("gitlab-ci:Pipeline")

	require.NotNil(t, meta)
	assert.Equal(t, "cicd", meta.SchemaType)
	assert.Equal(t, "gitlab.pipeline", meta.ResourceType)
	assert.Equal(t, 85, meta.CascadePriority)
	assert.Empty(t, meta.IdentityFields)
	assert.Equal(t, []string{"stages"}, meta.TrackedFields)
}

func TestRegisterGitLabPatterns(t *testing.T) {
	r := NewRegistry()
	RegisterGitLabPatterns(r)

	patterns := r.GetPatternsByEcosystem("gitlab")
	require.Len(t, patterns, 1)

	p := patterns[0]
	assert.Equal(t, "gitlab-ci", p.Name)
	assert.Equal(t, "gitlab", p.Ecosystem)
	assert.Equal(t, 70, p.Priority)
	assert.Empty(t, p.RequiredFields)
	assert.Contains(t, p.OptionalFields, "stages")
	assert.Contains(t, p.OptionalFields, "variables")
	assert.NotNil(t, p.TypeExtractor)
	assert.NotNil(t, p.ImportMapper)
	assert.NotNil(t, p.MetadataDefaults)
}

func TestGitLabPattern_Detection(t *testing.T) {
	r := NewRegistry()
	RegisterGitLabPatterns(r)

	tests := []struct {
		name           string
		data           map[string]interface{}
		expectMatch    bool
		expectedTypeID string
		expectedImport string
	}{
		{
			name: "valid pipeline with stages",
			data: map[string]interface{}{
				"stages": []interface{}{"build", "test"},
				"build": map[string]interface{}{
					"stage":  "build",
					"script": []interface{}{"make build"},
				},
				"test": map[string]interface{}{
					"stage":  "test",
					"script": []interface{}{"make test"},
				},
			},
			expectMatch:    true,
			expectedTypeID: "gitlab-ci:Pipeline",
			expectedImport: "cue.dev/x/gitlab/gitlabci:Pipeline",
		},
		{
			name: "valid pipeline without stages",
			data: map[string]interface{}{
				"build": map[string]interface{}{
					"script": []interface{}{"make build"},
				},
			},
			expectMatch:    true,
			expectedTypeID: "gitlab-ci:Pipeline",
			expectedImport: "cue.dev/x/gitlab/gitlabci:Pipeline",
		},
		{
			name: "pipeline with variables and workflow",
			data: map[string]interface{}{
				"variables": map[string]interface{}{
					"CI": "true",
				},
				"workflow": map[string]interface{}{
					"rules": []interface{}{},
				},
				"build": map[string]interface{}{
					"script": []interface{}{"make build"},
				},
			},
			expectMatch:    true,
			expectedTypeID: "gitlab-ci:Pipeline",
			expectedImport: "cue.dev/x/gitlab/gitlabci:Pipeline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Detect(tt.data)

			if !tt.expectMatch {
				// For non-match, either nil or empty TypeID
				if result != nil {
					assert.Empty(t, result.TypeID)
				}
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, "gitlab-ci", result.Pattern.Name)
			assert.Equal(t, tt.expectedTypeID, result.TypeID)
			assert.Equal(t, tt.expectedImport, result.ImportPath)
			assert.Equal(t, "Pipeline", result.Definition)
			assert.Greater(t, result.Confidence, 0.0)
		})
	}
}


func TestGitLabPattern_KubernetesHasHigherPriority(t *testing.T) {
	r := NewRegistry()
	RegisterKubernetesPatterns(r)
	RegisterGitLabPatterns(r)

	// Kubernetes data should match Kubernetes, not GitLab
	kubeData := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name": "test",
		},
		"spec": map[string]interface{}{},
	}

	result := r.Detect(kubeData)
	require.NotNil(t, result)
	assert.Equal(t, "kubernetes", result.Pattern.Name)
	assert.Equal(t, "v1:Pod", result.TypeID)
}

func TestGitLabPattern_DetectWithInclude(t *testing.T) {
	r := NewRegistry()
	RegisterGitLabPatterns(r)

	// Pipeline with include directive
	data := map[string]interface{}{
		"include": []interface{}{
			map[string]interface{}{
				"local": ".gitlab/ci/build.yml",
			},
			map[string]interface{}{
				"project": "my-group/my-project",
				"ref":     "main",
				"file":    "templates/ci.yml",
			},
		},
		"deploy": map[string]interface{}{
			"stage":  "deploy",
			"script": []interface{}{"deploy.sh"},
		},
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "gitlab-ci", result.Pattern.Name)
	assert.Equal(t, "gitlab-ci:Pipeline", result.TypeID)
	assert.Equal(t, "cue.dev/x/gitlab/gitlabci:Pipeline", result.ImportPath)
}

func TestGitLabPattern_ConfidenceWithOptionalFields(t *testing.T) {
	r := NewRegistry()
	RegisterGitLabPatterns(r)

	// Pipeline with optional fields should have higher confidence
	withOptional := map[string]interface{}{
		"stages": []interface{}{"build", "test"},
		"variables": map[string]interface{}{
			"CI": "true",
		},
		"build": map[string]interface{}{
			"script": []interface{}{"make build"},
		},
	}

	// Pipeline without optional fields
	withoutOptional := map[string]interface{}{
		"build": map[string]interface{}{
			"script": []interface{}{"make build"},
		},
	}

	resultWith := r.Detect(withOptional)
	resultWithout := r.Detect(withoutOptional)

	require.NotNil(t, resultWith)
	require.NotNil(t, resultWithout)

	// Both should be detected as GitLab CI
	assert.Equal(t, "gitlab-ci:Pipeline", resultWith.TypeID)
	assert.Equal(t, "gitlab-ci:Pipeline", resultWithout.TypeID)

	// With optional fields should have higher confidence
	assert.Greater(t, resultWith.Confidence, resultWithout.Confidence,
		"Pipeline with optional fields should have higher confidence")
}


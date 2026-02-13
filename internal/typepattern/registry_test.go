package typepattern

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.Empty(t, r.patterns)
}

func TestRegister(t *testing.T) {
	r := NewRegistry()

	p1 := &TypePattern{Name: "test1", Ecosystem: "test", Priority: 10}
	p2 := &TypePattern{Name: "test2", Ecosystem: "test", Priority: 20}
	p3 := &TypePattern{Name: "test3", Ecosystem: "test", Priority: 15}

	r.Register(p1)
	r.Register(p2)
	r.Register(p3)

	// Patterns should be sorted by priority (highest first)
	assert.Len(t, r.patterns, 3)
	assert.Equal(t, "test2", r.patterns[0].Name)
	assert.Equal(t, "test3", r.patterns[1].Name)
	assert.Equal(t, "test1", r.patterns[2].Name)
}

func TestDetect_NoPatterns(t *testing.T) {
	r := NewRegistry()
	data := map[string]interface{}{"key": "value"}

	result := r.Detect(data)
	assert.Nil(t, result)
}

func TestDetect_RequiredFieldsMissing(t *testing.T) {
	r := NewRegistry()
	r.Register(&TypePattern{
		Name:           "test",
		RequiredFields: []string{"apiVersion", "kind"},
	})

	data := map[string]interface{}{"apiVersion": "v1"}
	result := r.Detect(data)
	assert.Nil(t, result)
}

func TestDetect_RequiredFieldsPresent(t *testing.T) {
	r := NewRegistry()
	r.Register(&TypePattern{
		Name:           "kubernetes",
		Ecosystem:      "kubernetes",
		RequiredFields: []string{"apiVersion", "kind"},
	})

	data := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
	}
	result := r.Detect(data)

	require.NotNil(t, result)
	assert.Equal(t, "kubernetes", result.Pattern.Name)
	assert.GreaterOrEqual(t, result.Confidence, 0.5)
}

func TestDetect_OptionalFieldsBoostConfidence(t *testing.T) {
	r := NewRegistry()
	r.Register(&TypePattern{
		Name:           "kubernetes",
		RequiredFields: []string{"apiVersion", "kind"},
		OptionalFields: []string{"metadata", "spec"},
	})

	// Data without optional fields
	dataMinimal := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
	}
	resultMinimal := r.Detect(dataMinimal)
	require.NotNil(t, resultMinimal)

	// Data with optional fields
	dataFull := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": "test"},
		"spec":       map[string]interface{}{},
	}
	resultFull := r.Detect(dataFull)
	require.NotNil(t, resultFull)

	// Full data should have higher confidence
	assert.Greater(t, resultFull.Confidence, resultMinimal.Confidence)
}

func TestDetect_FieldValuesBoostConfidence(t *testing.T) {
	r := NewRegistry()
	r.Register(&TypePattern{
		Name:           "kubernetes",
		RequiredFields: []string{"apiVersion", "kind"},
		FieldValues: map[string][]string{
			"kind": {"Pod", "Deployment", "Service"},
		},
	})

	// Data with matching field value
	dataMatch := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
	}
	resultMatch := r.Detect(dataMatch)
	require.NotNil(t, resultMatch)

	// Data with non-matching field value
	dataNoMatch := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "CustomResource",
	}
	resultNoMatch := r.Detect(dataNoMatch)
	require.NotNil(t, resultNoMatch)

	assert.Greater(t, resultMatch.Confidence, resultNoMatch.Confidence)
}

func TestDetect_TypeExtractorAndImportMapper(t *testing.T) {
	r := NewRegistry()
	r.Register(&TypePattern{
		Name:           "kubernetes",
		RequiredFields: []string{"apiVersion", "kind"},
		TypeExtractor: func(data map[string]interface{}) string {
			return data["apiVersion"].(string) + ":" + data["kind"].(string)
		},
		ImportMapper: func(typeID string) string {
			return "cue.dev/x/k8s.io/api/" + typeID
		},
	})

	data := map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
	}
	result := r.Detect(data)

	require.NotNil(t, result)
	assert.Equal(t, "batch/v1:Job", result.TypeID)
	assert.Equal(t, "cue.dev/x/k8s.io/api/batch/v1:Job", result.ImportPath)
	assert.Equal(t, "Job", result.Definition)
}

func TestDetect_PriorityOrder(t *testing.T) {
	r := NewRegistry()

	// Lower priority pattern that would match
	r.Register(&TypePattern{
		Name:           "generic",
		Ecosystem:      "generic",
		RequiredFields: []string{"apiVersion"},
		Priority:       10,
	})

	// Higher priority pattern that would also match
	r.Register(&TypePattern{
		Name:           "kubernetes",
		Ecosystem:      "kubernetes",
		RequiredFields: []string{"apiVersion"},
		Priority:       100,
	})

	data := map[string]interface{}{"apiVersion": "v1"}
	result := r.Detect(data)

	require.NotNil(t, result)
	// Both patterns match with same confidence, but kubernetes was added to sorted list first
	// Since both have the same confidence (0.5), the first one checked wins
	assert.Equal(t, "kubernetes", result.Pattern.Name)
}

func TestGetPatternsByEcosystem(t *testing.T) {
	r := NewRegistry()

	r.Register(&TypePattern{Name: "k8s-pod", Ecosystem: "kubernetes", Priority: 10})
	r.Register(&TypePattern{Name: "k8s-deployment", Ecosystem: "kubernetes", Priority: 20})
	r.Register(&TypePattern{Name: "aws-ec2", Ecosystem: "aws", Priority: 15})
	r.Register(&TypePattern{Name: "gitlab-ci", Ecosystem: "gitlab", Priority: 5})

	// Get kubernetes patterns
	k8sPatterns := r.GetPatternsByEcosystem("kubernetes")
	assert.Len(t, k8sPatterns, 2)

	// Get AWS patterns
	awsPatterns := r.GetPatternsByEcosystem("aws")
	assert.Len(t, awsPatterns, 1)
	assert.Equal(t, "aws-ec2", awsPatterns[0].Name)

	// Get non-existent ecosystem
	noPatterns := r.GetPatternsByEcosystem("nonexistent")
	assert.Empty(t, noPatterns)
}

func TestExtractDefinition(t *testing.T) {
	tests := []struct {
		typeID   string
		expected string
	}{
		{"batch/v1:Job", "Job"},
		{"v1:Pod", "Pod"},
		{"Job", "Job"},
		{"apps/v1:Deployment", "Deployment"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.typeID, func(t *testing.T) {
			result := extractDefinition(tt.typeID)
			assert.Equal(t, tt.expected, result)
		})
	}
}


package inference

import (
	"testing"

	"pudl/internal/validator"
)

func TestExtractTopLevelFields(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		expected []string
	}{
		{
			name: "map with fields",
			data: map[string]interface{}{
				"InstanceId":   "i-12345",
				"State":        map[string]interface{}{"Name": "running"},
				"InstanceType": "t2.micro",
			},
			expected: []string{"InstanceId", "InstanceType", "State"},
		},
		{
			name: "array with first element map",
			data: []interface{}{
				map[string]interface{}{
					"kind":       "Pod",
					"apiVersion": "v1",
				},
			},
			expected: []string{"apiVersion", "kind"},
		},
		{
			name:     "empty map",
			data:     map[string]interface{}{},
			expected: []string{},
		},
		{
			name:     "nil",
			data:     nil,
			expected: []string{},
		},
		{
			name:     "non-map data",
			data:     "just a string",
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := GetFieldList(tc.data)

			if len(fields) != len(tc.expected) {
				t.Errorf("Expected %d fields, got %d: %v", len(tc.expected), len(fields), fields)
				return
			}

			for i, field := range tc.expected {
				if fields[i] != field {
					t.Errorf("Expected field[%d] = %s, got %s", i, field, fields[i])
				}
			}
		})
	}
}

func TestScoreCandidate(t *testing.T) {
	dataFields := map[string]bool{
		"InstanceId":   true,
		"State":        true,
		"InstanceType": true,
		"Tags":         true,
	}

	tests := []struct {
		name       string
		schemaName string
		meta       validator.SchemaMetadata
		hints      InferenceHints
		minScore   float64
		maxScore   float64
	}{
		{
			name:       "all identity fields match",
			schemaName: "aws.#EC2Instance",
			meta: validator.SchemaMetadata{
				IdentityFields: []string{"InstanceId"},
				TrackedFields:  []string{"State", "InstanceType", "Tags"},
			},
			hints:    InferenceHints{},
			minScore: 0.5, // identity field match
			maxScore: 1.0,
		},
		{
			name:       "identity field missing",
			schemaName: "aws.#S3Bucket",
			meta: validator.SchemaMetadata{
				IdentityFields: []string{"Name"},
				TrackedFields:  []string{"CreationDate"},
			},
			hints:    InferenceHints{},
			minScore: 0.0,
			maxScore: 0.1,
		},
		{
			name:       "origin matches resource type",
			schemaName: "aws.#EC2Instance",
			meta: validator.SchemaMetadata{
				ResourceType:   "aws.ec2.instance",
				IdentityFields: []string{"InstanceId"},
			},
			hints:    InferenceHints{Origin: "aws-ec2-describe-instances"},
			minScore: 0.5, // identity + origin match
			maxScore: 1.0,
		},
		{
			name:       "catchall always scores",
			schemaName: "core.#Item",
			meta: validator.SchemaMetadata{
				SchemaType: "catchall",
			},
			hints:    InferenceHints{},
			minScore: 0.01,
			maxScore: 0.1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score, _ := scoreCandidate(tc.schemaName, tc.meta, dataFields, tc.hints)

			if score < tc.minScore {
				t.Errorf("Score %f below minimum %f", score, tc.minScore)
			}
			if score > tc.maxScore {
				t.Errorf("Score %f above maximum %f", score, tc.maxScore)
			}
		})
	}
}

func TestSelectCandidates(t *testing.T) {
	metadata := map[string]validator.SchemaMetadata{
		"core.#Item": {
			SchemaType:      "catchall",
			CascadePriority: 0,
		},
		"aws.#Resource": {
			ResourceType:    "aws.resource",
			CascadePriority: 50,
		},
		"aws.#EC2Instance": {
			ResourceType:    "aws.ec2.instance",
			BaseSchema:      "aws.#Resource",
			CascadePriority: 80,
			IdentityFields:  []string{"InstanceId"},
			TrackedFields:   []string{"State", "InstanceType"},
		},
		"k8s.#Pod": {
			ResourceType:    "k8s.pod",
			CascadePriority: 80,
			IdentityFields:  []string{"kind", "apiVersion", "metadata"},
		},
	}

	graph := BuildInheritanceGraph(metadata)

	t.Run("EC2 instance data", func(t *testing.T) {
		data := map[string]interface{}{
			"InstanceId":   "i-12345",
			"State":        map[string]interface{}{"Name": "running"},
			"InstanceType": "t2.micro",
		}

		candidates := SelectCandidates(data, InferenceHints{Origin: "aws-ec2"}, metadata, graph)

		if len(candidates) == 0 {
			t.Fatal("Expected at least one candidate")
		}

		// EC2Instance should be the top candidate
		if candidates[0].Schema != "aws.#EC2Instance" {
			t.Errorf("Expected aws.#EC2Instance as top candidate, got %s", candidates[0].Schema)
		}
	})

	t.Run("K8s Pod data", func(t *testing.T) {
		data := map[string]interface{}{
			"kind":       "Pod",
			"apiVersion": "v1",
			"metadata":   map[string]interface{}{"name": "test-pod"},
		}

		candidates := SelectCandidates(data, InferenceHints{}, metadata, graph)

		if len(candidates) == 0 {
			t.Fatal("Expected at least one candidate")
		}

		// Pod should be the top candidate
		if candidates[0].Schema != "k8s.#Pod" {
			t.Errorf("Expected k8s.#Pod as top candidate, got %s", candidates[0].Schema)
		}
	})

	t.Run("Unknown data includes catchall", func(t *testing.T) {
		data := map[string]interface{}{
			"foo": "bar",
			"baz": 123,
		}

		candidates := SelectCandidates(data, InferenceHints{}, metadata, graph)

		// Should have at least catchall
		hasCatchall := false
		for _, c := range candidates {
			if c.Schema == "core.#Item" {
				hasCatchall = true
				break
			}
		}
		if !hasCatchall {
			t.Error("Expected catchall to be in candidates")
		}
	})
}

func TestSortCandidates(t *testing.T) {
	metadata := map[string]validator.SchemaMetadata{
		"schemaA": {CascadePriority: 100},
		"schemaB": {CascadePriority: 50, BaseSchema: "schemaA"},
		"schemaC": {CascadePriority: 75},
	}
	graph := BuildInheritanceGraph(metadata)

	candidates := []CandidateScore{
		{Schema: "schemaC", Score: 0.5},
		{Schema: "schemaA", Score: 0.5},
		{Schema: "schemaB", Score: 0.5},
	}

	sortCandidates(candidates, graph)

	// schemaB has depth 1, others have depth 0
	// So schemaB should come first despite lower priority
	if candidates[0].Schema != "schemaB" {
		t.Errorf("Expected schemaB first (highest depth), got %s", candidates[0].Schema)
	}

	// Among depth-0 schemas, schemaA (priority 100) should beat schemaC (priority 75)
	if candidates[1].Schema != "schemaA" {
		t.Errorf("Expected schemaA second (higher priority), got %s", candidates[1].Schema)
	}
}

func TestOriginMatching(t *testing.T) {
	dataFields := map[string]bool{"foo": true}

	tests := []struct {
		origin       string
		resourceType string
		shouldMatch  bool
		description  string
	}{
		// Requires at least 2 parts to match
		{"aws-ec2-instances", "aws.ec2.instance", true, "aws+ec2+instance all match"},
		{"aws-ec2", "aws.ec2.instance", true, "aws+ec2 both match"},
		{"kubectl-pods", "k8s.pod", false, "only pod matches (need 2)"},
		{"k8s-pods", "k8s.pod", true, "k8s+pod both match"},
		{"aws-s3-buckets", "aws.ec2.instance", false, "only aws matches"},
		{"", "aws.ec2.instance", false, "empty origin"},
		{"aws-ec2", "", false, "empty resource type"},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			meta := validator.SchemaMetadata{
				ResourceType: tc.resourceType,
			}
			hints := InferenceHints{Origin: tc.origin}

			score, _ := scoreCandidate("test", meta, dataFields, hints)

			if tc.shouldMatch && score < 0.1 {
				t.Errorf("Expected origin match, got score %f", score)
			}
			if !tc.shouldMatch && score >= 0.15 {
				t.Errorf("Expected no origin match, got score %f", score)
			}
		})
	}
}

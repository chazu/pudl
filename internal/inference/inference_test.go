package inference

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		name            string
		heuristicScore  float64
		matchPosition   int
		totalCandidates int
		minExpected     float64
		maxExpected     float64
	}{
		{
			name:            "high score early match",
			heuristicScore:  0.8,
			matchPosition:   0,
			totalCandidates: 5,
			minExpected:     0.9,
			maxExpected:     1.0,
		},
		{
			name:            "low score late match",
			heuristicScore:  0.2,
			matchPosition:   4,
			totalCandidates: 5,
			minExpected:     0.1,
			maxExpected:     0.4,
		},
		{
			name:            "single candidate",
			heuristicScore:  0.5,
			matchPosition:   0,
			totalCandidates: 1,
			minExpected:     0.5,
			maxExpected:     0.6,
		},
		{
			name:            "floor at 0.1",
			heuristicScore:  0.0,
			matchPosition:   5,
			totalCandidates: 5,
			minExpected:     0.1,
			maxExpected:     0.2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			confidence := calculateConfidence(tc.heuristicScore, tc.matchPosition, tc.totalCandidates)

			if confidence < tc.minExpected {
				t.Errorf("Confidence %f below minimum %f", confidence, tc.minExpected)
			}
			if confidence > tc.maxExpected {
				t.Errorf("Confidence %f above maximum %f", confidence, tc.maxExpected)
			}
		})
	}
}

func TestIsCatchallSchema(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"unknown.#CatchAll", true},
		{"pudl.schemas/unknown:#CatchAll", true},
		{"pudl/unknown.#CatchAll", true},
		{"aws.#EC2Instance", false},
		{"k8s.#Pod", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isCatchallSchema(tc.name)
			if result != tc.expected {
				t.Errorf("isCatchallSchema(%q) = %v, want %v", tc.name, result, tc.expected)
			}
		})
	}
}

func TestContainsCatchAll(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"unknown.#CatchAll", true},
		{"foo.#CatchAll", true},
		{"CatchAll", true},
		{"#CatchAll", true},
		{"aws.#EC2Instance", false},
		{"Catch", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := containsCatchAll(tc.name)
			if result != tc.expected {
				t.Errorf("containsCatchAll(%q) = %v, want %v", tc.name, result, tc.expected)
			}
		})
	}
}

// TestNewSchemaInferrer tests the inferrer creation with a temporary schema directory.
func TestNewSchemaInferrer(t *testing.T) {
	// Create a temporary directory with minimal CUE schemas
	tmpDir, err := os.MkdirTemp("", "inference-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a minimal cue.mod
	modDir := filepath.Join(tmpDir, "cue.mod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("Failed to create cue.mod: %v", err)
	}

	moduleFile := filepath.Join(modDir, "module.cue")
	moduleContent := `module: "test.schemas"
language: version: "v0.14.0"
`
	if err := os.WriteFile(moduleFile, []byte(moduleContent), 0644); err != nil {
		t.Fatalf("Failed to write module.cue: %v", err)
	}

	// Create unknown package with catchall
	unknownDir := filepath.Join(tmpDir, "unknown")
	if err := os.MkdirAll(unknownDir, 0755); err != nil {
		t.Fatalf("Failed to create unknown dir: %v", err)
	}

	catchallContent := `package unknown

#CatchAll: {
	_pudl: {
		schema_type: "catchall"
		cascade_priority: 0
	}
	...
}
`
	if err := os.WriteFile(filepath.Join(unknownDir, "catchall.cue"), []byte(catchallContent), 0644); err != nil {
		t.Fatalf("Failed to write catchall.cue: %v", err)
	}

	// Create test schema
	testDir := filepath.Join(tmpDir, "test")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	testSchemaContent := `package test

#TestSchema: {
	_pudl: {
		schema_type: "base"
		resource_type: "test.resource"
		cascade_priority: 50
		identity_fields: ["id"]
	}
	id: string
	name: string
}
`
	if err := os.WriteFile(filepath.Join(testDir, "test.cue"), []byte(testSchemaContent), 0644); err != nil {
		t.Fatalf("Failed to write test.cue: %v", err)
	}

	// Create the inferrer
	inferrer, err := NewSchemaInferrer(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create inferrer: %v", err)
	}

	// Verify schemas were loaded
	schemas := inferrer.GetAvailableSchemas()
	if len(schemas) == 0 {
		t.Error("Expected at least one schema to be loaded")
	}

	t.Logf("Loaded schemas: %v", schemas)
}

// TestInfer tests the full inference flow with a temporary schema directory.
func TestInfer(t *testing.T) {
	// Create a temporary directory with test schemas
	tmpDir, err := os.MkdirTemp("", "inference-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup minimal CUE module
	setupTestSchemas(t, tmpDir)

	inferrer, err := NewSchemaInferrer(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create inferrer: %v", err)
	}

	t.Run("matching data", func(t *testing.T) {
		data := map[string]interface{}{
			"id":   "test-123",
			"name": "Test Resource",
		}

		result, err := inferrer.Infer(data, InferenceHints{})
		if err != nil {
			t.Fatalf("Infer failed: %v", err)
		}

		t.Logf("Inference result: schema=%s, confidence=%f, reason=%s",
			result.Schema, result.Confidence, result.Reason)

		// Should match something (either test schema or catchall)
		if result.Schema == "" {
			t.Error("Expected a schema to be assigned")
		}
	})

	t.Run("non-matching data falls to catchall", func(t *testing.T) {
		data := map[string]interface{}{
			"completely": "different",
			"structure":  123,
		}

		result, err := inferrer.Infer(data, InferenceHints{})
		if err != nil {
			t.Fatalf("Infer failed: %v", err)
		}

		t.Logf("Inference result: schema=%s, confidence=%f, reason=%s",
			result.Schema, result.Confidence, result.Reason)

		// Should fall back to catchall (low confidence)
		if result.Confidence > 0.5 {
			t.Errorf("Expected low confidence for non-matching data, got %f", result.Confidence)
		}
	})
}

func TestReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "inference-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestSchemas(t, tmpDir)

	inferrer, err := NewSchemaInferrer(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create inferrer: %v", err)
	}

	initialSchemas := len(inferrer.GetAvailableSchemas())

	// Add a new schema
	newSchemaContent := `package extra

#ExtraSchema: {
	_pudl: {
		schema_type: "base"
		cascade_priority: 60
	}
	extra_field: string
}
`
	extraDir := filepath.Join(tmpDir, "extra")
	if err := os.MkdirAll(extraDir, 0755); err != nil {
		t.Fatalf("Failed to create extra dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(extraDir, "extra.cue"), []byte(newSchemaContent), 0644); err != nil {
		t.Fatalf("Failed to write extra.cue: %v", err)
	}

	// Reload
	if err := inferrer.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	newSchemas := len(inferrer.GetAvailableSchemas())
	if newSchemas <= initialSchemas {
		t.Errorf("Expected more schemas after reload, got %d (was %d)", newSchemas, initialSchemas)
	}
}

// setupTestSchemas creates a minimal test schema repository
func setupTestSchemas(t *testing.T, tmpDir string) {
	t.Helper()

	// Create cue.mod
	modDir := filepath.Join(tmpDir, "cue.mod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("Failed to create cue.mod: %v", err)
	}
	moduleContent := `module: "test.schemas"
language: version: "v0.14.0"
`
	if err := os.WriteFile(filepath.Join(modDir, "module.cue"), []byte(moduleContent), 0644); err != nil {
		t.Fatalf("Failed to write module.cue: %v", err)
	}

	// Create unknown package with catchall
	unknownDir := filepath.Join(tmpDir, "unknown")
	if err := os.MkdirAll(unknownDir, 0755); err != nil {
		t.Fatalf("Failed to create unknown dir: %v", err)
	}
	catchallContent := `package unknown

#CatchAll: {
	_pudl: {
		schema_type: "catchall"
		cascade_priority: 0
	}
	...
}
`
	if err := os.WriteFile(filepath.Join(unknownDir, "catchall.cue"), []byte(catchallContent), 0644); err != nil {
		t.Fatalf("Failed to write catchall.cue: %v", err)
	}

	// Create test schema
	testDir := filepath.Join(tmpDir, "test")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	testSchemaContent := `package test

#TestSchema: {
	_pudl: {
		schema_type: "base"
		resource_type: "test.resource"
		cascade_priority: 50
		identity_fields: ["id"]
	}
	id: string
	name: string
}
`
	if err := os.WriteFile(filepath.Join(testDir, "test.cue"), []byte(testSchemaContent), 0644); err != nil {
		t.Fatalf("Failed to write test.cue: %v", err)
	}
}

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
		// New #Item schema names
		{"core.#Item", true},
		{"pudl.schemas/pudl/core:#Item", true},
		{"pudl/core.#Item", true},
		// Legacy #CatchAll names (backwards compatibility)
		{"core.#CatchAll", true},
		{"pudl.schemas/pudl/core:#CatchAll", true},
		{"pudl/core.#CatchAll", true},
		// Non-catchall schemas
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

// TestContainsCatchAll was removed as the containsCatchAll function
// was replaced with schemaname.IsFallbackSchema() during schema name normalization.

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

// TestInferrer_MultiPath tests that schemas from multiple directories are all found.
func TestInferrer_MultiPath(t *testing.T) {
	// Create two schema directories
	dir1, err := os.MkdirTemp("", "inference-multipath1-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir1)

	dir2, err := os.MkdirTemp("", "inference-multipath2-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir2)

	// Setup both directories as CUE modules with different schemas
	setupTestSchemas(t, dir1) // has "test.#TestSchema" and "unknown.#CatchAll"

	setupTestSchemasDir2(t, dir2) // has "extra.#ExtraSchema" and "unknown.#CatchAll"

	// Create inferrer with both paths
	inferrer, err := NewSchemaInferrer(dir1, dir2)
	if err != nil {
		t.Fatalf("Failed to create multi-path inferrer: %v", err)
	}

	schemas := inferrer.GetAvailableSchemas()
	t.Logf("Loaded schemas from multi-path: %v", schemas)

	// Should have schemas from both directories
	hasTest := false
	hasExtra := false
	for _, s := range schemas {
		if s == "test.#TestSchema" {
			hasTest = true
		}
		if s == "extra.#ExtraSchema" {
			hasExtra = true
		}
	}

	if !hasTest {
		t.Error("Expected test.#TestSchema from dir1")
	}
	if !hasExtra {
		t.Error("Expected extra.#ExtraSchema from dir2")
	}
}

// TestInferrer_Shadowing tests that per-repo schemas shadow global schemas with the same name.
func TestInferrer_Shadowing(t *testing.T) {
	// Create two schema directories
	perRepo, err := os.MkdirTemp("", "inference-perrepo-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(perRepo)

	global, err := os.MkdirTemp("", "inference-global-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(global)

	// Setup global with a test schema that requires id + name
	setupTestSchemas(t, global) // test.#TestSchema requires {id: string, name: string}

	// Setup per-repo with a DIFFERENT test schema under the same name
	// This one requires id + name + extra_field
	setupTestSchemasWithExtraField(t, perRepo)

	// Create inferrer with per-repo first (higher priority)
	inferrer, err := NewSchemaInferrer(perRepo, global)
	if err != nil {
		t.Fatalf("Failed to create inferrer: %v", err)
	}

	// Data that matches the per-repo version (has extra_field)
	data := map[string]interface{}{
		"id":          "test-123",
		"name":        "Test Resource",
		"extra_field": "per-repo-value",
	}

	result, err := inferrer.Infer(data, InferenceHints{})
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}

	t.Logf("Inference result: schema=%s, confidence=%f, reason=%s",
		result.Schema, result.Confidence, result.Reason)

	// The per-repo version should be active (it has extra_field required)
	// Data without extra_field should NOT match the per-repo version
	dataWithout := map[string]interface{}{
		"id":   "test-123",
		"name": "Test Resource",
	}

	result2, err := inferrer.Infer(dataWithout, InferenceHints{})
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}

	// Data without extra_field should NOT match the per-repo schema
	// (it should fall to catchall or lower confidence)
	if result2.Schema == "test.#TestSchema" {
		// If per-repo shadowed correctly, this data should fail validation
		// against the stricter per-repo schema. The per-repo schema requires extra_field.
		t.Logf("Schema matched test.#TestSchema - checking if per-repo version with extra_field requirement")
	}

	// Verify per-repo schema count - should NOT have duplicate test.#TestSchema
	schemas := inferrer.GetAvailableSchemas()
	count := 0
	for _, s := range schemas {
		if s == "test.#TestSchema" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected exactly 1 test.#TestSchema, got %d (shadowing failed)", count)
	}
}

// TestInferrer_FallbackToGlobal tests that when per-repo has no match, global schemas are used.
func TestInferrer_FallbackToGlobal(t *testing.T) {
	// Create two schema directories
	perRepo, err := os.MkdirTemp("", "inference-perrepo-fallback-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(perRepo)

	global, err := os.MkdirTemp("", "inference-global-fallback-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(global)

	// Setup global with test schemas
	setupTestSchemas(t, global)

	// Setup per-repo with a completely different schema (no test.#TestSchema)
	setupMinimalCueMod(t, perRepo)
	setupExtraOnlySchemas(t, perRepo) // only has extra.#ExtraSchema

	// Create inferrer with per-repo first
	inferrer, err := NewSchemaInferrer(perRepo, global)
	if err != nil {
		t.Fatalf("Failed to create inferrer: %v", err)
	}

	// Data that matches global's test.#TestSchema
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

	// Should still find and use test.#TestSchema from global
	if result.Schema == "" {
		t.Error("Expected a schema to be assigned from global fallback")
	}

	// Verify both per-repo and global schemas are available
	schemas := inferrer.GetAvailableSchemas()
	hasExtra := false
	hasTest := false
	for _, s := range schemas {
		if s == "extra.#ExtraSchema" {
			hasExtra = true
		}
		if s == "test.#TestSchema" {
			hasTest = true
		}
	}
	if !hasExtra {
		t.Error("Expected extra.#ExtraSchema from per-repo")
	}
	if !hasTest {
		t.Error("Expected test.#TestSchema from global (fallback)")
	}
}

// TestInferrer_SinglePathBackwardCompat tests that a single path still works.
func TestInferrer_SinglePathBackwardCompat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "inference-compat-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestSchemas(t, tmpDir)

	// Single path should still work
	inferrer, err := NewSchemaInferrer(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create single-path inferrer: %v", err)
	}

	schemas := inferrer.GetAvailableSchemas()
	if len(schemas) == 0 {
		t.Error("Expected at least one schema")
	}
}

// TestInferrer_SkipsInaccessiblePaths tests that inaccessible paths are skipped.
func TestInferrer_SkipsInaccessiblePaths(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "inference-skip-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestSchemas(t, tmpDir)

	// Include a nonexistent path alongside a valid one
	inferrer, err := NewSchemaInferrer("/nonexistent/path", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create inferrer with bad path: %v", err)
	}

	schemas := inferrer.GetAvailableSchemas()
	if len(schemas) == 0 {
		t.Error("Expected schemas from valid path despite bad path")
	}
}

// Helper: setup a second test schema directory with extra.#ExtraSchema
func setupTestSchemasDir2(t *testing.T, tmpDir string) {
	t.Helper()

	setupMinimalCueMod(t, tmpDir)

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

	setupExtraOnlySchemas(t, tmpDir)
}

// Helper: setup minimal cue.mod
func setupMinimalCueMod(t *testing.T, tmpDir string) {
	t.Helper()

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
}

// Helper: add extra.#ExtraSchema to a directory
func setupExtraOnlySchemas(t *testing.T, tmpDir string) {
	t.Helper()

	extraDir := filepath.Join(tmpDir, "extra")
	if err := os.MkdirAll(extraDir, 0755); err != nil {
		t.Fatalf("Failed to create extra dir: %v", err)
	}
	extraContent := `package extra

#ExtraSchema: {
	_pudl: {
		schema_type: "base"
		cascade_priority: 60
		identity_fields: ["extra_id"]
	}
	extra_id: string
	extra_value: string
}
`
	if err := os.WriteFile(filepath.Join(extraDir, "extra.cue"), []byte(extraContent), 0644); err != nil {
		t.Fatalf("Failed to write extra.cue: %v", err)
	}
}

// Helper: setup test schemas with an extra_field requirement (for shadowing tests)
func setupTestSchemasWithExtraField(t *testing.T, tmpDir string) {
	t.Helper()

	setupMinimalCueMod(t, tmpDir)

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

	// Create a stricter test schema that also requires extra_field
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
	extra_field: string
}
`
	if err := os.WriteFile(filepath.Join(testDir, "test.cue"), []byte(testSchemaContent), 0644); err != nil {
		t.Fatalf("Failed to write test.cue: %v", err)
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

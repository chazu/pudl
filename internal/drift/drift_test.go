package drift

import (
	"testing"
)

func TestCompare_Identical(t *testing.T) {
	declared := map[string]interface{}{
		"name":   "test",
		"count":  float64(3),
		"active": true,
	}
	live := map[string]interface{}{
		"name":   "test",
		"count":  float64(3),
		"active": true,
	}

	diffs := Compare(declared, live)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d: %+v", len(diffs), diffs)
	}
}

func TestCompare_Changed(t *testing.T) {
	declared := map[string]interface{}{
		"name":  "old",
		"count": float64(3),
	}
	live := map[string]interface{}{
		"name":  "new",
		"count": float64(5),
	}

	diffs := Compare(declared, live)
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d: %+v", len(diffs), diffs)
	}

	// Sorted by path
	assertDiff(t, diffs[0], "count", "changed", float64(3), float64(5))
	assertDiff(t, diffs[1], "name", "changed", "old", "new")
}

func TestCompare_Added(t *testing.T) {
	declared := map[string]interface{}{
		"name": "test",
	}
	live := map[string]interface{}{
		"name":  "test",
		"extra": "surprise",
	}

	diffs := Compare(declared, live)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	assertDiff(t, diffs[0], "extra", "added", nil, "surprise")
}

func TestCompare_Removed(t *testing.T) {
	declared := map[string]interface{}{
		"name":   "test",
		"region": "us-east-1",
	}
	live := map[string]interface{}{
		"name": "test",
	}

	diffs := Compare(declared, live)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	assertDiff(t, diffs[0], "region", "removed", "us-east-1", nil)
}

func TestCompare_Nested(t *testing.T) {
	declared := map[string]interface{}{
		"config": map[string]interface{}{
			"port":    float64(8080),
			"host":    "localhost",
			"timeout": float64(30),
		},
	}
	live := map[string]interface{}{
		"config": map[string]interface{}{
			"port": float64(9090),
			"host": "localhost",
			"tls":  true,
		},
	}

	diffs := Compare(declared, live)
	if len(diffs) != 3 {
		t.Fatalf("expected 3 diffs, got %d: %+v", len(diffs), diffs)
	}

	assertDiff(t, diffs[0], "config.port", "changed", float64(8080), float64(9090))
	assertDiff(t, diffs[1], "config.timeout", "removed", float64(30), nil)
	assertDiff(t, diffs[2], "config.tls", "added", nil, true)
}

func TestCompare_NumericCoercion(t *testing.T) {
	declared := map[string]interface{}{
		"count": 3, // int
	}
	live := map[string]interface{}{
		"count": float64(3), // float64 from JSON
	}

	diffs := Compare(declared, live)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs with numeric coercion, got %d: %+v", len(diffs), diffs)
	}
}

func TestCompare_EmptyMaps(t *testing.T) {
	diffs := Compare(map[string]interface{}{}, map[string]interface{}{})
	if len(diffs) != 0 {
		t.Errorf("expected no diffs for empty maps, got %d", len(diffs))
	}
}

func TestCompare_DeeplyNested(t *testing.T) {
	declared := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "deep",
			},
		},
	}
	live := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "changed",
			},
		},
	}

	diffs := Compare(declared, live)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	assertDiff(t, diffs[0], "a.b.c", "changed", "deep", "changed")
}

func assertDiff(t *testing.T, diff FieldDiff, path, diffType string, declared, live interface{}) {
	t.Helper()
	if diff.Path != path {
		t.Errorf("expected path %q, got %q", path, diff.Path)
	}
	if diff.Type != diffType {
		t.Errorf("expected type %q, got %q", diffType, diff.Type)
	}
	if !valuesEqual(diff.Declared, declared) {
		t.Errorf("expected declared %v, got %v", declared, diff.Declared)
	}
	if !valuesEqual(diff.Live, live) {
		t.Errorf("expected live %v, got %v", live, diff.Live)
	}
}

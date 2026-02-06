package identity

import (
	"testing"
)

func TestComputeResourceID_Deterministic(t *testing.T) {
	values := map[string]interface{}{"id": "abc-123"}
	id1 := ComputeResourceID("aws/ec2.#Instance", values, "")
	id2 := ComputeResourceID("aws/ec2.#Instance", values, "")

	if id1 != id2 {
		t.Errorf("expected deterministic IDs, got %s and %s", id1, id2)
	}
}

func TestComputeResourceID_SameSchemaAndValues(t *testing.T) {
	values := map[string]interface{}{"id": "abc-123", "region": "us-east-1"}
	id1 := ComputeResourceID("aws/ec2.#Instance", values, "")
	id2 := ComputeResourceID("aws/ec2.#Instance", values, "")

	if id1 != id2 {
		t.Errorf("same schema + same values should produce same ID")
	}
}

func TestComputeResourceID_DifferentValues(t *testing.T) {
	values1 := map[string]interface{}{"id": "abc-123"}
	values2 := map[string]interface{}{"id": "def-456"}
	id1 := ComputeResourceID("aws/ec2.#Instance", values1, "")
	id2 := ComputeResourceID("aws/ec2.#Instance", values2, "")

	if id1 == id2 {
		t.Error("different values should produce different IDs")
	}
}

func TestComputeResourceID_DifferentSchemas(t *testing.T) {
	values := map[string]interface{}{"id": "abc-123"}
	id1 := ComputeResourceID("aws/ec2.#Instance", values, "")
	id2 := ComputeResourceID("k8s/v1.#Pod", values, "")

	if id1 == id2 {
		t.Error("different schemas should produce different IDs")
	}
}

func TestComputeResourceID_CatchallUsesHash(t *testing.T) {
	// Empty identity values (catchall) should use content hash
	id1 := ComputeResourceID("pudl/core.#Item", nil, "deadbeef1234")
	id2 := ComputeResourceID("pudl/core.#Item", nil, "deadbeef5678")

	if id1 == id2 {
		t.Error("catchall with different content hashes should produce different IDs")
	}
}

func TestComputeResourceID_CatchallEmptyMap(t *testing.T) {
	// Empty map (not nil) should also be treated as catchall
	id1 := ComputeResourceID("pudl/core.#Item", map[string]interface{}{}, "deadbeef1234")
	id2 := ComputeResourceID("pudl/core.#Item", nil, "deadbeef1234")

	if id1 != id2 {
		t.Error("nil and empty map identity values should produce same ID for catchall")
	}
}

func TestComputeResourceID_SchemaNormalization(t *testing.T) {
	values := map[string]interface{}{"id": "abc-123"}
	// These should all normalize to the same schema
	id1 := ComputeResourceID("aws/ec2.#Instance", values, "")
	id2 := ComputeResourceID("pudl.schemas/aws/ec2@v0:#Instance", values, "")

	if id1 != id2 {
		t.Errorf("equivalent schema names should produce same ID, got %s and %s", id1, id2)
	}
}

func TestComputeResourceID_KeyOrderDoesntMatter(t *testing.T) {
	// Different insertion order, same logical content
	values1 := map[string]interface{}{"id": "abc", "region": "us-east-1"}
	values2 := map[string]interface{}{"region": "us-east-1", "id": "abc"}
	id1 := ComputeResourceID("aws/ec2.#Instance", values1, "")
	id2 := ComputeResourceID("aws/ec2.#Instance", values2, "")

	if id1 != id2 {
		t.Error("key order should not affect resource ID")
	}
}

func TestComputeResourceID_Length(t *testing.T) {
	values := map[string]interface{}{"id": "abc-123"}
	id := ComputeResourceID("aws/ec2.#Instance", values, "")

	if len(id) != 64 {
		t.Errorf("expected 64-char hex SHA256, got %d chars: %s", len(id), id)
	}
}

func TestCanonicalIdentityJSON_SortedKeys(t *testing.T) {
	values := map[string]interface{}{"z": 1, "a": 2, "m": 3}
	result, err := CanonicalIdentityJSON(values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"a":2,"m":3,"z":1}`
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestCanonicalIdentityJSON_Empty(t *testing.T) {
	result, err := CanonicalIdentityJSON(map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "{}" {
		t.Errorf("expected {}, got %s", result)
	}
}

func TestCanonicalIdentityJSON_Deterministic(t *testing.T) {
	values := map[string]interface{}{"name": "test", "id": "123"}
	r1, _ := CanonicalIdentityJSON(values)
	r2, _ := CanonicalIdentityJSON(values)
	if r1 != r2 {
		t.Error("expected deterministic output")
	}
}

func TestCanonicalIdentityJSON_StringValues(t *testing.T) {
	values := map[string]interface{}{"id": "abc-123"}
	result, err := CanonicalIdentityJSON(values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"id":"abc-123"}`
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

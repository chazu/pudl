package identity

import (
	"testing"
)

func TestExtractFieldValues_FlatField(t *testing.T) {
	data := map[string]interface{}{
		"id":   "abc-123",
		"name": "test",
	}

	result, err := ExtractFieldValues(data, []string{"id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "abc-123" {
		t.Errorf("expected id=abc-123, got %v", result["id"])
	}
}

func TestExtractFieldValues_NestedPath(t *testing.T) {
	data := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "my-pod",
			"namespace": "default",
		},
	}

	result, err := ExtractFieldValues(data, []string{"metadata.name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["metadata.name"] != "my-pod" {
		t.Errorf("expected metadata.name=my-pod, got %v", result["metadata.name"])
	}
}

func TestExtractFieldValues_MultipleFields(t *testing.T) {
	data := map[string]interface{}{
		"id":     "abc-123",
		"region": "us-east-1",
		"type":   "t2.micro",
	}

	result, err := ExtractFieldValues(data, []string{"id", "region"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "abc-123" {
		t.Errorf("expected id=abc-123, got %v", result["id"])
	}
	if result["region"] != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %v", result["region"])
	}
}

func TestExtractFieldValues_CompositeKey(t *testing.T) {
	data := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "my-pod",
			"namespace": "default",
		},
		"kind": "Pod",
	}

	result, err := ExtractFieldValues(data, []string{"metadata.name", "metadata.namespace"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["metadata.name"] != "my-pod" {
		t.Errorf("expected metadata.name=my-pod, got %v", result["metadata.name"])
	}
	if result["metadata.namespace"] != "default" {
		t.Errorf("expected metadata.namespace=default, got %v", result["metadata.namespace"])
	}
}

func TestExtractFieldValues_MissingField(t *testing.T) {
	data := map[string]interface{}{
		"id": "abc-123",
	}

	_, err := ExtractFieldValues(data, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestExtractFieldValues_ArrayInput(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{
			"id":   "first",
			"name": "item-1",
		},
		map[string]interface{}{
			"id":   "second",
			"name": "item-2",
		},
	}

	result, err := ExtractFieldValues(data, []string{"id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should extract from first element
	if result["id"] != "first" {
		t.Errorf("expected id=first, got %v", result["id"])
	}
}

func TestExtractFieldValues_EmptyArray(t *testing.T) {
	data := []interface{}{}

	_, err := ExtractFieldValues(data, []string{"id"})
	if err == nil {
		t.Fatal("expected error for empty array")
	}
}

func TestExtractFieldValues_EmptyFields(t *testing.T) {
	data := map[string]interface{}{
		"id": "abc-123",
	}

	result, err := ExtractFieldValues(data, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestExtractFieldValues_NilFields(t *testing.T) {
	data := map[string]interface{}{
		"id": "abc-123",
	}

	result, err := ExtractFieldValues(data, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestExtractFieldValues_DeeplyNested(t *testing.T) {
	data := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "deep-value",
			},
		},
	}

	result, err := ExtractFieldValues(data, []string{"a.b.c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["a.b.c"] != "deep-value" {
		t.Errorf("expected a.b.c=deep-value, got %v", result["a.b.c"])
	}
}

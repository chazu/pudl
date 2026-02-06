package identity

import (
	"fmt"
	"strings"
)

// ExtractFieldValues extracts values from parsed JSON data for the given field paths.
// Supports dot-notation for nested paths (e.g., "metadata.name").
// Returns map[field_path]value. Returns error if any field is missing.
// For arrays, extracts from the first element.
// Empty fields slice returns an empty map (valid for catchall schemas).
func ExtractFieldValues(data interface{}, fields []string) (map[string]interface{}, error) {
	if len(fields) == 0 {
		return map[string]interface{}{}, nil
	}

	// For arrays, extract from the first element
	if arr, ok := data.([]interface{}); ok {
		if len(arr) == 0 {
			return nil, fmt.Errorf("cannot extract identity fields from empty array")
		}
		data = arr[0]
	}

	result := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		val, found := extractNestedField(data, field)
		if !found {
			return nil, fmt.Errorf("identity field %q not found in data", field)
		}
		result[field] = val
	}

	return result, nil
}

// extractNestedField traverses nested maps using dot-separated path.
func extractNestedField(data interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")

	current := data
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, exists := m[part]
		if !exists {
			return nil, false
		}
		current = val
	}

	return current, true
}

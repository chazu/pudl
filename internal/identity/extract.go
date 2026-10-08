package identity

import (
	"fmt"

	"github.com/chazu/pudl/internal/fieldpath"
)

// ExtractFieldValues extracts values from parsed JSON data for the given field paths.
// Paths use the fieldpath syntax: dot-notation for nested fields (e.g.
// "metadata.name"), with quoted segments for keys that contain dots (e.g.
// `metadata.labels."cloud.googleapis.com/location"`).
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
		path, err := fieldpath.Parse(field)
		if err != nil {
			return nil, fmt.Errorf("identity field: %w", err)
		}
		if path.HasWildcard() {
			return nil, fmt.Errorf("identity field %q: wildcards are not allowed in identity fields", field)
		}
		val, found := path.Lookup(data)
		if !found {
			return nil, fmt.Errorf("identity field %q not found in data", field)
		}
		result[field] = val
	}

	return result, nil
}

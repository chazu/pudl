package systemmodel

import "github.com/chazu/pudl/internal/schemaname"

// SchemaName is the catalog schema a recorded model instance is assigned.
const SchemaName = "pudl/systemmodel.#SystemModel"

// WithoutLegacyEmptyPopulate returns a stored model instance with the empty
// `populate: {}` that older pudl versions recorded for checks-only models
// removed, and true when it did so. That key meant "no populate arm" but does
// not satisfy #SystemModel. Any other payload is returned unchanged with false.
func WithoutLegacyEmptyPopulate(schema string, data any) (any, bool) {
	if !schemaname.IsEquivalent(schema, SchemaName) {
		return data, false
	}
	record, ok := data.(map[string]any)
	if !ok {
		return data, false
	}
	populate, ok := record["populate"].(map[string]any)
	if !ok || len(populate) != 0 {
		return data, false
	}
	stripped := make(map[string]any, len(record)-1)
	for key, value := range record {
		if key != "populate" {
			stripped[key] = value
		}
	}
	return stripped, true
}

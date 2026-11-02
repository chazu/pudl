package inference

// discriminatorBonus outranks a generic schema that matches on field names
// alone: identity plus tracked fields score at most 0.6, so a schema whose
// identity fields and pinned value (e.g. kind: "compute#firewall") both match
// is tried before one that merely shares names like `name` and `kind`.
const discriminatorBonus = 0.3

// matchesDiscriminator reports whether the record carries the exact value of
// at least one field the schema pins to a single string.
func matchesDiscriminator(discriminators map[string]string, data interface{}) bool {
	record, ok := data.(map[string]interface{})
	if !ok {
		return false
	}
	for field, want := range discriminators {
		if got, ok := record[field].(string); ok && got == want {
			return true
		}
	}
	return false
}

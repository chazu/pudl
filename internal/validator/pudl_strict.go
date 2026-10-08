package validator

import (
	"fmt"

	"cuelang.org/go/cue"
)

// decodeStrictPudlParts reads the `_pudl` fields whose mistakes must surface
// rather than vanish: sensitive_fields (redaction) and facts (projection).
// Anything present but not concrete and well-typed is recorded as an error on
// the metadata; consumers fail closed (redaction) or disable the feature with a
// warning (projection).
func decodeStrictPudlParts(pudl cue.Value, meta *SchemaMetadata) {
	if v := pudl.LookupPath(cue.ParsePath("sensitive_fields")); v.Exists() {
		var fields []string
		if err := v.Validate(cue.Concrete(true)); err != nil {
			meta.SensitiveError = fmt.Sprintf("sensitive_fields: %v", err)
		} else if err := v.Decode(&fields); err != nil {
			meta.SensitiveError = fmt.Sprintf("sensitive_fields must be a list of strings: %v", err)
		} else {
			meta.SensitiveFields = fields
		}
	}
	if v := pudl.LookupPath(cue.ParsePath("sensitive_mode")); v.Exists() {
		// Only redaction exists; a mode field is a mistake (e.g. a hash mode
		// someone expected) and must not be mistaken for protection.
		meta.SensitiveError = "sensitive_mode is not supported: sensitive fields are always redacted"
	}
	if v := pudl.LookupPath(cue.ParsePath("facts")); v.Exists() {
		if err := v.Validate(cue.Concrete(true)); err != nil {
			meta.FactsError = fmt.Sprintf("facts: %v", err)
		} else if raw, err := v.MarshalJSON(); err != nil {
			meta.FactsError = fmt.Sprintf("facts: %v", err)
		} else {
			meta.FactsSpec = raw
		}
	}
}

package validator

import (
	"fmt"

	"github.com/chazu/pudl/internal/schemaname"
)

// SchemaTypePolicy is the `_pudl.schema_type` of a policy schema: a
// specialization of a type that says what a compliant instance looks like.
// Policies are evaluated against resources and never assigned as a record's
// schema, so they take no part in classification or chain fallback.
const SchemaTypePolicy = "policy"

// IsPolicy reports whether the schema is a policy rather than a type.
func (m SchemaMetadata) IsPolicy() bool {
	return m.SchemaType == SchemaTypePolicy
}

// NearestType walks schema's base_schema chain and returns the first schema
// that is not a policy: schema itself when it is a type, otherwise the type
// its policy chain specializes. It returns "" when the chain reaches no loaded
// type (a missing base schema or a cycle).
func NearestType(schema string, lookup func(string) (SchemaMetadata, bool)) string {
	seen := map[string]bool{}
	for current := schema; current != "" && !seen[current]; {
		seen[current] = true
		meta, ok := lookup(current)
		if !ok {
			return ""
		}
		if !meta.IsPolicy() {
			return current
		}
		current = meta.BaseSchema
	}
	return ""
}

// PolicySchemaError reports an explicit schema (import --schema, a model's
// populate.schema) that names a policy instead of a type.
type PolicySchemaError struct {
	Policy string
	// Type is the policy's nearest type, or "" when its chain reaches none.
	Type string
}

func (e *PolicySchemaError) Error() string {
	if e.Type == "" {
		return fmt.Sprintf("%s is a policy schema, not a type, and its base_schema chain reaches no type; policies are never assigned to records", e.Policy)
	}
	return fmt.Sprintf("%s is a policy schema, not a type: name its type %s instead; compliance with %s is evaluated separately (`pudl policy evaluate`, a follow-up)", e.Policy, e.Type, e.Policy)
}

// CheckAssignable returns a *PolicySchemaError when schema is a loaded policy,
// and nil otherwise. Only types may be assigned to records.
func (cv *ChainValidator) CheckAssignable(schema string) error {
	schema = schemaname.Normalize(schema)
	meta, ok := cv.metadata[schema]
	if !ok || !meta.IsPolicy() {
		return nil
	}
	return &PolicySchemaError{Policy: schema, Type: NearestType(schema, cv.GetSchemaMetadata)}
}

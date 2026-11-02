package validator

import "cuelang.org/go/cue"

// literalStringFields returns the schema's required top-level fields that are
// pinned to a single string, such as `kind: "compute#firewall"`. A record that
// carries the same value is strong evidence for the schema, which inference
// scores as a discriminator. Fields with a default, disjunctions and open
// types are not discriminators; nil when the schema pins none.
func literalStringFields(schema cue.Value) map[string]string {
	iter, err := schema.Fields()
	if err != nil {
		return nil
	}
	var out map[string]string
	for iter.Next() {
		v := iter.Value()
		if _, hasDefault := v.Default(); hasDefault {
			continue
		}
		if v.Kind() != cue.StringKind {
			continue
		}
		s, err := v.String()
		if err != nil {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[iter.Selector().Unquoted()] = s
	}
	return out
}

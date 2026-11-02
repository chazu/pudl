package validator

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/fieldpath"
)

// MatchRule is one field a type marks with `@pudl(match)`: its path in the
// record and the schema's constraint on it.
type MatchRule struct {
	Path       fieldpath.Path
	Constraint cue.Value
}

// decodeMatch collects the type's `@pudl(match)` fields: required fields, at
// any struct depth, whose schema constraint only this type's records satisfy
// (a pinned kind, a name pattern). A record whose values at all of them
// satisfy their constraints claims the type: inference assigns it the type
// (or a descendant) or rejects it, and never falls back to a generic schema.
// A malformed attribute records MatchError, which CheckModuleIntegrity
// reports as a load error.
func decodeMatch(schema cue.Value, meta *SchemaMetadata) {
	rules, err := matchRules(schema, nil)
	if err != nil {
		meta.MatchError = fmt.Sprintf("@pudl(match): %v", err)
		return
	}
	meta.Match = rules
}

func matchRules(v cue.Value, prefix []string) ([]MatchRule, error) {
	if v.IncompleteKind()&cue.StructKind == 0 {
		return nil, nil
	}
	iter, err := v.Fields()
	if err != nil {
		return nil, nil
	}
	var rules []MatchRule
	for iter.Next() {
		keys := append(append([]string(nil), prefix...), iter.Selector().Unquoted())
		field := iter.Value()
		marked, err := hasMatchFlag(field)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(keys, "."), err)
		}
		if marked {
			rules = append(rules, MatchRule{Path: fieldpath.FromKeys(keys...), Constraint: field})
			continue
		}
		nested, err := matchRules(field, keys)
		if err != nil {
			return nil, err
		}
		rules = append(rules, nested...)
	}
	return rules, nil
}

// hasMatchFlag reports a bare `match` among the field's @pudl arguments.
func hasMatchFlag(v cue.Value) (bool, error) {
	for _, attr := range v.Attributes(cue.FieldAttr) {
		if attr.Name() != "pudl" {
			continue
		}
		if err := attr.Err(); err != nil {
			return false, err
		}
		for i := 0; i < attr.NumArgs(); i++ {
			if key, val := attr.Arg(i); key == "match" {
				if val != "" {
					return false, fmt.Errorf("match takes no value (got match=%q)", val)
				}
				return true, nil
			}
		}
	}
	return false, nil
}

// Claims reports whether data is a record whose values at every
// `@pudl(match)` field satisfy the schema's constraint there. A schema with
// no match fields claims nothing.
func (m SchemaMetadata) Claims(data interface{}) bool {
	if len(m.Match) == 0 {
		return false
	}
	if _, ok := data.(map[string]interface{}); !ok {
		return false
	}
	for _, rule := range m.Match {
		value, ok := rule.Path.Lookup(data)
		if !ok {
			return false
		}
		got := rule.Constraint.Context().Encode(value)
		if rule.Constraint.Unify(got).Validate(cue.Concrete(true)) != nil {
			return false
		}
	}
	return true
}

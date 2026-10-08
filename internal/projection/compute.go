package projection

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

// Result is what projecting one record produced.
type Result struct {
	Facts []database.Fact
	// PerRelation counts facts per relation, including zeros, so a caller can
	// warn about a declared relation that never yields anything.
	PerRelation map[string]int
	// Omitted describes arg values left out because the query layer could not
	// represent them (e.g. an integer beyond int64).
	Omitted []string
}

// Compute projects one record. data must already be redacted (see
// SchemaSpec.Sensitive). Facts carry the implicit entry_id and resource_id
// args; their Source and ValidStart are set by the reconcile.
func Compute(spec *SchemaSpec, entryID, resourceID string, data any) Result {
	res := Result{PerRelation: map[string]int{}}
	if spec == nil || spec.Err != nil {
		return res
	}
	for _, rel := range spec.Relations {
		res.PerRelation[rel.Name] += 0
		rows := []any{data}
		if rel.Each != nil {
			rows = rel.Each.LookupAll(data)
		}
		for _, row := range rows {
			for _, args := range rowArgs(rel, row, &res) {
				args[ArgEntryID] = entryID
				args[ArgResourceID] = resourceID
				encoded, err := json.Marshal(args)
				if err != nil {
					res.Omitted = append(res.Omitted, fmt.Sprintf("%s: %v", rel.Name, err))
					continue
				}
				res.Facts = append(res.Facts, database.Fact{
					Relation:   rel.Name,
					Args:       string(encoded),
					Provenance: fmt.Sprintf(`{"entry_id":%q,"schema":%q}`, entryID, spec.Schema),
				})
				res.PerRelation[rel.Name]++
			}
		}
	}
	return res
}

// rowArgs returns the arg maps one row yields: one, or one per value of the
// relation's wildcard arg. A wildcard arg with no values and no default yields
// none.
func rowArgs(rel RelationSpec, row any, res *Result) []map[string]any {
	base := map[string]any{}
	for _, name := range rel.ArgNames {
		if name == rel.wildcard {
			continue
		}
		spec := rel.Args[name]
		if spec.Exists {
			base[name] = len(spec.Path.LookupAll(row)) > 0
			continue
		}
		v, ok := spec.Path.Lookup(row)
		if !ok {
			if !spec.HasDefault {
				continue
			}
			v = spec.Default
		}
		if v, ok := admit(rel.Name, name, spec.trim(v), res); ok {
			base[name] = v
		}
	}
	if rel.wildcard == "" {
		return []map[string]any{base}
	}
	spec := rel.Args[rel.wildcard]
	values := spec.Path.LookupAll(row)
	if len(values) == 0 && spec.HasDefault {
		values = []any{spec.Default}
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		v, ok := admit(rel.Name, rel.wildcard, spec.trim(value), res)
		if !ok {
			continue
		}
		args := make(map[string]any, len(base)+3)
		for k, bv := range base {
			args[k] = bv
		}
		args[rel.wildcard] = v
		out = append(out, args)
	}
	return out
}

func (a ArgSpec) trim(v any) any {
	if s, ok := v.(string); ok && a.TrimPrefix != "" {
		return strings.TrimPrefix(s, a.TrimPrefix)
	}
	return v
}

// admit checks a value against the query layer's numeric contract: one value
// it cannot represent would make every query touching the relation fail, so
// such a value is omitted and reported instead.
func admit(relation, arg string, v any, res *Result) (any, bool) {
	if err := queryRepresentable(v); err != nil {
		res.Omitted = append(res.Omitted, fmt.Sprintf("%s.%s: %v", relation, arg, err))
		return nil, false
	}
	return v, true
}

func queryRepresentable(v any) error {
	switch t := v.(type) {
	case json.Number:
		_, err := database.QueryNumber(t)
		return err
	case map[string]any:
		for _, x := range t {
			if err := queryRepresentable(x); err != nil {
				return err
			}
		}
	case []any:
		for _, x := range t {
			if err := queryRepresentable(x); err != nil {
				return err
			}
		}
	}
	return nil
}

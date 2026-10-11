package projection

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
)

// Diagnostic explains why a check cannot establish its claim.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Schema  string `json:"schema,omitempty"`
	EntryID string `json:"entry_id,omitempty"`
}

// Dependencies includes the root and every relation reachable through rules.
func Dependencies(rules []datalog.Rule, root string) map[string]bool {
	out := map[string]bool{}
	var visit func(string)
	visit = func(rel string) {
		if out[rel] {
			return
		}
		out[rel] = true
		for _, r := range rules {
			if r.Head.Rel == rel {
				for _, a := range r.Body {
					visit(a.Rel)
				}
			}
		}
	}
	visit(root)
	return out
}

// RelationSchemas retains ownership even when a declaration fails to parse.
// Otherwise a broken spec with old stored facts would appear healthy.
func (r *Registry) RelationSchemas() map[string][]string {
	out := map[string][]string{}
	for _, schema := range r.Schemas() {
		seen := map[string]bool{}
		for _, parent := range r.src.GetInheritanceGraph().GetCascadeChain(schema) {
			meta, ok := r.src.GetSchemaMetadata(parent)
			if !ok {
				continue
			}
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(meta.FactsSpec, &raw)
			for name := range raw {
				if !seen[name] {
					out[name] = append(out[name], schema)
					seen[name] = true
				}
			}
		}
	}
	return out
}

// CheckDiagnostics inspects only the check's dependency closure. A declared
// empty relation is valid; an undeclared relation is missing evidence.
func CheckDiagnostics(db *database.CatalogDB, reg *Registry, rules []datalog.Rule, root string, syncReport SyncReport, syncErr error) []Diagnostic {
	deps := Dependencies(rules, root)
	stored := map[string]bool{}
	relations, err := db.GetDistinctRelations()
	if err != nil {
		return []Diagnostic{{Code: "evidence_unavailable", Message: err.Error()}}
	}
	for _, rel := range relations {
		stored[rel] = true
	}
	declared := reg.RelationArgs()
	owners := reg.RelationSchemas()
	heads := map[string]bool{}
	for _, r := range rules {
		heads[r.Head.Rel] = true
	}
	var out []Diagnostic
	for _, message := range Lint(rules, []string{root}, declared, stored) {
		out = append(out, Diagnostic{Code: "invalid_relation", Message: message})
	}
	needed := map[string]bool{}
	for rel := range deps {
		for _, schema := range owners[rel] {
			needed[schema] = true
		}
		if !heads[rel] && !stored[rel] && len(owners[rel]) == 0 && !database.IsReservedRelation(rel) {
			out = append(out, Diagnostic{Code: "missing_relation", Message: fmt.Sprintf("relation %s has no declared or recorded evidence", rel)})
		}
	}
	if len(needed) > 0 && syncErr != nil {
		out = append(out, Diagnostic{Code: "projection_sync_failed", Message: syncErr.Error()})
	}
	for schema := range needed {
		if spec := reg.For(schema); spec != nil && spec.Err != nil {
			out = append(out, Diagnostic{Code: "invalid_projection", Schema: schema, Message: spec.Err.Error()})
		}
		for _, message := range syncReport.Failures[schema] {
			out = append(out, Diagnostic{Code: "projection_failed", Schema: schema, Message: message})
		}
	}
	states, err := db.ListProjectionState()
	if err != nil && len(needed) > 0 {
		out = append(out, Diagnostic{Code: "evidence_unavailable", Message: err.Error()})
	}
	for _, state := range states {
		if needed[state.Schema] && state.Status == database.ProjectionIncomplete {
			out = append(out, Diagnostic{Code: "projection_incomplete", Schema: state.Schema, EntryID: state.EntryID, Message: "projected values were omitted because they are outside the query domain; inspect the source record and projection"})
		}
		if needed[state.Schema] && state.Status == database.ProjectionSkipped {
			out = append(out, Diagnostic{Code: "identity_unresolved", Schema: state.Schema, EntryID: state.EntryID, Message: "record has no resolved resource identity"})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Code+out[i].Schema+out[i].Message+out[i].EntryID < out[j].Code+out[j].Schema+out[j].Message+out[j].EntryID
	})
	return out
}

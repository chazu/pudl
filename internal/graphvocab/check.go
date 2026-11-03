package graphvocab

import (
	"encoding/json"
	"fmt"

	"github.com/chazu/pudl/internal/datalog"
)

// crossCheck reports references the vocabulary cannot resolve: an edge type
// in an unknown category, a role naming an undeclared edge type, a
// graph_edge rule deriving an undeclared type. A provider with no
// graph_provider fact is not a problem: pudl ui labels it with its id.
func crossCheck(v *Vocabulary, rules []datalog.Rule) []string {
	var problems []string
	categories := map[string]bool{}
	for _, c := range v.Categories {
		categories[c.ID] = true
	}
	edgeTypes := map[string]bool{}
	for _, e := range v.EdgeTypes {
		edgeTypes[e.Type] = true
		if !categories[e.Category] {
			problems = append(problems, fmt.Sprintf("edge type %s (%s): category %q is not declared by %s; pudl ui shows it as other", e.Type, e.Source, e.Category, CategoryRelation))
		}
	}
	for _, rt := range v.ResourceTypes {
		where := rt.Schema
		if where == "" {
			where = rt.Source
		}
		for _, t := range rt.EdgeTypes() {
			if !edgeTypes[t] {
				problems = append(problems, fmt.Sprintf("resource type %s (%s): role names edge type %q, which no %s fact declares", rt.Type, where, t, EdgeTypeRelation))
			}
		}
		if rt.Code == "" {
			problems = append(problems, fmt.Sprintf("resource type %s (%s): graph roles but no display {code, label, class}", rt.Type, where))
		}
	}
	for _, r := range rules {
		if r.Head.Rel != EdgeRelation || !r.Valid() {
			continue
		}
		term, ok := r.Head.Args["type"]
		if !ok || term.IsVariable() {
			continue
		}
		if t := scalar(term.Value); t != "" && !edgeTypes[t] {
			problems = append(problems, fmt.Sprintf("rule %s derives graph_edge type %q, which no %s fact declares; pudl ui shows it as other", r.Describe(), t, EdgeTypeRelation))
		}
	}
	return problems
}

func rolesKey(r any) string {
	b, _ := json.Marshal(r)
	return string(b)
}

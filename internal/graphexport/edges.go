package graphexport

import (
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/datalog"
)

// RuleTuples are the graph_edge tuples one rule derived.
type RuleTuples struct {
	Rule   datalog.Rule
	Tuples []datalog.Tuple
}

// refIndex resolves endpoint values to resource IDs.
type refIndex struct {
	ids  map[string]bool
	refs map[string][]string
}

func newRefIndex() *refIndex {
	return &refIndex{ids: map[string]bool{}, refs: map[string][]string{}}
}

func (x *refIndex) add(resourceID string, refs []string) {
	x.ids[resourceID] = true
	for _, ref := range refs {
		if !contains(x.refs[ref], resourceID) {
			x.refs[ref] = append(x.refs[ref], resourceID)
		}
	}
}

// resolve maps an endpoint to a resource ID, or to RefPrefix+value when it
// names no cataloged resource (or more than one).
func (x *refIndex) resolve(value string) (id string, problem string) {
	if x.ids[value] {
		return value, ""
	}
	switch matches := x.refs[value]; len(matches) {
	case 1:
		return matches[0], ""
	case 0:
		return RefPrefix + value, ""
	default:
		return RefPrefix + value, fmt.Sprintf("reference %q matches %d resources; left unresolved", value, len(matches))
	}
}

// buildRelationships turns rule tuples into relationships. Duplicates (the
// same type, source and target from several rules) merge their evidence.
func buildRelationships(rules []RuleTuples, index *refIndex, warn func(string)) []Relationship {
	byID := map[string]*Relationship{}
	for _, rt := range rules {
		nullRows := 0
		for _, t := range rt.Tuples {
			// An absent optional field binds as null (an unencrypted volume
			// has no key): that row means "no edge", not an error.
			if t.Args["source"] == nil || t.Args["target"] == nil {
				nullRows++
				continue
			}
			typ, okT := stringArg(t, "type")
			src, okS := stringArg(t, "source")
			tgt, okG := stringArg(t, "target")
			basis, _ := stringArg(t, "basis")
			if !okT || !okS || !okG {
				warn(fmt.Sprintf("rule %s: graph_edge row without string type, source and target: %v", rt.Rule.Describe(), t.Args))
				continue
			}
			if !Bases[basis] {
				warn(fmt.Sprintf("rule %s: basis %q is not observed, declared, inferred or unknown; using unknown", rt.Rule.Describe(), basis))
				basis = "unknown"
			}
			srcID, p1 := index.resolve(src)
			tgtID, p2 := index.resolve(tgt)
			for _, p := range []string{p1, p2} {
				if p != "" {
					warn(fmt.Sprintf("rule %s: %s", rt.Rule.Describe(), p))
				}
			}
			ev := Evidence{Basis: basis, Source: "rule " + rt.Rule.Name, Detail: rt.Rule.Source}
			id := fmt.Sprintf("%s:%s->%s", typ, srcID, tgtID)
			if rel, ok := byID[id]; ok {
				if !containsEvidence(rel.Evidence, ev) {
					rel.Evidence = append(rel.Evidence, ev)
				}
				continue
			}
			byID[id] = &Relationship{ID: id, Type: typ, Source: srcID, Target: tgtID, Basis: basis, Evidence: []Evidence{ev}}
		}
		// Only null rows usually means the rule names an arg the facts lack.
		if nullRows > 0 && nullRows == len(rt.Tuples) {
			warn(fmt.Sprintf("rule %s: all %d rows have a null source or target; check its arg names", rt.Rule.Describe(), nullRows))
		}
	}
	out := make([]Relationship, 0, len(byID))
	for _, rel := range byID {
		out = append(out, *rel)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func stringArg(t datalog.Tuple, key string) (string, bool) {
	s, ok := t.Args[key].(string)
	return s, ok && s != ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func containsEvidence(list []Evidence, e Evidence) bool {
	for _, v := range list {
		if v == e {
			return true
		}
	}
	return false
}

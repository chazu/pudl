package graphexport

import (
	"sort"
	"strings"
)

// Subgraph returns the part of g around focus: the focus resources, every
// resource within depth hops over any relationship, and, from all of those,
// the resources reached by following the closure relationship types
// outward (containment, so the client can still place what it shows).
//
// Only relationships whose two ends are both returned are included, plus
// relationships to unresolved references (ref:...), which are terminal. A
// client therefore never mistakes "not loaded" for "not in the inventory":
// Meta.Degree gives each returned resource's total relationship count, and
// the difference is what is not loaded. Meta.Owned does the same for what
// an owner owns (ownership is an attribute, not a relationship).
func Subgraph(g *Graph, focus []string, depth int, closure []string) *Graph {
	known := map[string]bool{}
	for _, r := range g.Resources {
		known[r.ID] = true
	}
	adj := map[string][]string{}
	degree := map[string]int{}
	follow := map[string]bool{}
	for _, t := range closure {
		follow[t] = true
	}
	up := map[string][]string{} // closure edges, source -> target
	for _, rel := range g.Relationships {
		degree[rel.Source]++
		degree[rel.Target]++
		if known[rel.Source] && known[rel.Target] {
			adj[rel.Source] = append(adj[rel.Source], rel.Target)
			adj[rel.Target] = append(adj[rel.Target], rel.Source)
		}
		if follow[rel.Type] && known[rel.Target] {
			up[rel.Source] = append(up[rel.Source], rel.Target)
		}
	}

	aliases := map[string]string{}
	for _, r := range g.Resources {
		for _, rep := range r.Representations {
			aliases[rep.ID] = r.ID
		}
	}
	in := map[string]bool{}
	frontier := []string{}
	for _, id := range focus {
		if group, ok := aliases[id]; ok {
			id = group // a grouped member's ID focuses its group
		}
		if known[id] && !in[id] {
			in[id] = true
			frontier = append(frontier, id)
		}
	}
	for d := 0; d < depth; d++ {
		var next []string
		for _, id := range frontier {
			for _, n := range adj[id] {
				if !in[n] {
					in[n] = true
					next = append(next, n)
				}
			}
		}
		frontier = next
	}
	// Containment closure, transitively (subnet -> VPC -> ...).
	stack := make([]string, 0, len(in))
	for id := range in {
		stack = append(stack, id)
	}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, p := range up[id] {
			if !in[p] {
				in[p] = true
				stack = append(stack, p)
			}
		}
	}

	out := &Graph{Meta: g.Meta, Resources: []Resource{}, Relationships: []Relationship{}, Vocabulary: g.Vocabulary}
	out.Meta.Partial = true
	out.Meta.Totals = &Totals{Resources: len(g.Resources), Relationships: len(g.Relationships)}
	out.Meta.Degree = map[string]int{}
	owned := ownedCounts(g)
	for _, r := range g.Resources {
		if in[r.ID] {
			out.Resources = append(out.Resources, r)
			out.Meta.Degree[r.ID] = degree[r.ID]
			if n := owned[r.ID]; n > 0 {
				if out.Meta.Owned == nil {
					out.Meta.Owned = map[string]int{}
				}
				out.Meta.Owned[r.ID] = n
			}
		}
	}
	for _, rel := range g.Relationships {
		srcOK := in[rel.Source] || strings.HasPrefix(rel.Source, RefPrefix)
		tgtOK := in[rel.Target] || strings.HasPrefix(rel.Target, RefPrefix)
		if srcOK && tgtOK && (in[rel.Source] || in[rel.Target]) {
			out.Relationships = append(out.Relationships, rel)
		}
	}
	return out
}

// SearchHit is a lightweight resource match.
type SearchHit struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Provider string `json:"provider"`
	Account  string `json:"account,omitempty"`
	Region   string `json:"region,omitempty"`
	NativeID string `json:"nativeId,omitempty"`
}

// Search finds resources whose name, native ID, type or ID contains q
// (case-insensitive). See searchRank for the order.
func Search(g *Graph, q string, limit int) []SearchHit {
	q = strings.ToLower(strings.TrimSpace(q))
	hits := []SearchHit{}
	if q == "" {
		return hits
	}
	type scored struct {
		hit  SearchHit
		rank int
	}
	var found []scored
	for _, r := range g.Resources {
		if rank := searchRank(r, q); rank >= 0 {
			found = append(found, scored{SearchHit{r.ID, r.Name, r.Type, r.Provider, r.Account, r.Region, r.NativeID}, rank})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if len(a.hit.Name) != len(b.hit.Name) {
			return len(a.hit.Name) < len(b.hit.Name)
		}
		return a.hit.Name < b.hit.Name
	})
	for i, f := range found {
		if i == limit {
			break
		}
		hits = append(hits, f.hit)
	}
	return hits
}

// searchRank orders matches for a lower-cased query: exact name, name
// prefix, name substring, exact native ID (or its last path segment),
// native ID substring, then type or ID. -1: no match. The web UI ranks its
// local search the same way (web/src/ui/searchRank.ts).
func searchRank(r Resource, q string) int {
	name, native := strings.ToLower(r.Name), strings.ToLower(r.NativeID)
	switch {
	case name == q:
		return 0
	case strings.HasPrefix(name, q):
		return 1
	case strings.Contains(name, q):
		return 2
	case native == q, strings.HasSuffix(native, "/"+q):
		return 3
	case strings.Contains(native, q):
		return 4
	case strings.Contains(strings.ToLower(r.Type), q), strings.Contains(strings.ToLower(r.ID), q):
		return 5
	}
	return -1
}

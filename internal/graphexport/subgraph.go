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
// the difference is what is not loaded.
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
	for _, r := range g.Resources {
		if in[r.ID] {
			out.Resources = append(out.Resources, r)
			out.Meta.Degree[r.ID] = degree[r.ID]
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
// (case-insensitive). Name matches rank first.
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
		rank := -1
		switch {
		case strings.Contains(strings.ToLower(r.Name), q):
			rank = 0
		case strings.Contains(strings.ToLower(r.NativeID), q):
			rank = 1
		case strings.Contains(strings.ToLower(r.Type), q), strings.Contains(strings.ToLower(r.ID), q):
			rank = 2
		}
		if rank >= 0 {
			found = append(found, scored{SearchHit{r.ID, r.Name, r.Type, r.Provider, r.Account, r.Region, r.NativeID}, rank})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].rank != found[j].rank {
			return found[i].rank < found[j].rank
		}
		return found[i].hit.Name < found[j].hit.Name
	})
	for i, f := range found {
		if i == limit {
			break
		}
		hits = append(hits, f.hit)
	}
	return hits
}

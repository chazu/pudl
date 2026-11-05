package sameresource

import "sort"

// Link is one same_resource row: resources A and B represent one resource of
// Kind.
type Link struct {
	A, B, Kind string
}

// Group is a set of resources that represent one real resource. ID is the
// smallest member resource_id, so it changes only when membership does.
type Group struct {
	ID      string
	Members []string // sorted
	Kinds   []string // sorted, distinct
}

// Conflict is a connected component that holds two or more resources of one
// identity namespace. Two resources of one schema family with the same key
// are not two views of one thing (a collision, or a stale record), so the
// component is not formed and its members stay standalone.
type Conflict struct {
	Members []string // the whole component, sorted
	Kinds   []string // sorted, distinct
	// Namespaces maps each namespace holding more than one member to those
	// members (sorted).
	Namespaces map[string][]string
}

// Build groups resources linked directly or transitively. Self links are
// ignored. namespaceOf returns a resource's identity namespace (its schema
// family root); an empty namespace never conflicts. Groups and conflicts are
// sorted by their first member.
func Build(links []Link, namespaceOf func(resourceID string) string) ([]Group, []Conflict) {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == x {
			return x
		}
		root := find(parent[x])
		parent[x] = root
		return root
	}
	add := func(x string) {
		if _, ok := parent[x]; !ok {
			parent[x] = x
		}
	}
	for _, l := range links {
		if l.A == l.B || l.A == "" || l.B == "" {
			continue
		}
		add(l.A)
		add(l.B)
		ra, rb := find(l.A), find(l.B)
		if ra != rb {
			if rb < ra {
				ra, rb = rb, ra
			}
			parent[rb] = ra
		}
	}

	members := map[string][]string{}
	for x := range parent {
		root := find(x)
		members[root] = append(members[root], x)
	}
	kinds := map[string]map[string]bool{}
	for _, l := range links {
		if l.A == l.B || l.A == "" || l.B == "" {
			continue
		}
		root := find(l.A)
		if kinds[root] == nil {
			kinds[root] = map[string]bool{}
		}
		kinds[root][l.Kind] = true
	}

	var groups []Group
	var conflicts []Conflict
	for root, ids := range members {
		sort.Strings(ids)
		kindList := sortedKeys(kinds[root])
		if dup := duplicateNamespaces(ids, namespaceOf); len(dup) > 0 {
			conflicts = append(conflicts, Conflict{Members: ids, Kinds: kindList, Namespaces: dup})
			continue
		}
		groups = append(groups, Group{ID: ids[0], Members: ids, Kinds: kindList})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Members[0] < conflicts[j].Members[0] })
	return groups, conflicts
}

// duplicateNamespaces returns the namespaces holding more than one of ids.
func duplicateNamespaces(ids []string, namespaceOf func(string) string) map[string][]string {
	byNamespace := map[string][]string{}
	for _, id := range ids {
		ns := ""
		if namespaceOf != nil {
			ns = namespaceOf(id)
		}
		if ns != "" {
			byNamespace[ns] = append(byNamespace[ns], id)
		}
	}
	dup := map[string][]string{}
	for ns, nsIDs := range byNamespace {
		if len(nsIDs) > 1 {
			dup[ns] = nsIDs // ids is sorted, so nsIDs is too
		}
	}
	if len(dup) == 0 {
		return nil
	}
	return dup
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

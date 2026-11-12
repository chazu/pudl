package graphexport

import (
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/datalog"
)

// dropMemberOnly removes resources whose schema is member_only and that no
// group joined to a record of another schema. It returns the survivors and
// how many records of each schema it dropped.
func dropMemberOnly(items []builtResource, meta MetadataFunc) ([]builtResource, map[string]int) {
	dropped := map[string]int{}
	out := items[:0]
	for _, it := range items {
		if len(it.r.Representations) == 0 && memberOnly(it.r.Schema, meta) {
			dropped[it.r.Schema]++
			continue
		}
		out = append(out, it)
	}
	return out, dropped
}

func memberOnly(schema string, meta MetadataFunc) bool {
	m, ok := meta(schema)
	return ok && m.Graph != nil && m.Graph.MemberOnly
}

// applyDetails attaches graph_detail rows to the resources they name. A row
// needs resource_id, section, key and a non-null value; rank is optional.
// Rows naming no resource in the graph are counted and reported.
func applyDetails(resources []Resource, rows []RuleTuples, index *refIndex, warn func(string)) {
	byID := map[string]*Resource{}
	for i := range resources {
		byID[resources[i].ID] = &resources[i]
	}
	orphans := 0
	seen := map[string]map[string]bool{}
	for _, rt := range rows {
		for _, t := range rt.Tuples {
			rid, okR := stringArg(t, "resource_id")
			section, okS := stringArg(t, "section")
			key, okK := stringArg(t, "key")
			value, present := t.Args["value"]
			if !okR || !okS || !okK || !present || value == nil {
				continue
			}
			id, _ := index.resolve(rid)
			r := byID[id]
			if r == nil {
				orphans++
				continue
			}
			d := Detail{Section: section, Key: key, Value: value, Rank: intArg(t, "rank")}
			sig := fmt.Sprintf("%s|%s|%d|%v", section, key, d.Rank, value)
			if seen[id] == nil {
				seen[id] = map[string]bool{}
			}
			if seen[id][sig] {
				continue
			}
			seen[id][sig] = true
			r.Details = append(r.Details, d)
		}
	}
	if orphans > 0 {
		warn(fmt.Sprintf("%d graph_detail rows name a resource that is not in the graph", orphans))
	}
	for _, r := range byID {
		sort.Slice(r.Details, func(i, j int) bool {
			a, b := r.Details[i], r.Details[j]
			if a.Section != b.Section {
				return a.Section < b.Section
			}
			if a.Key != b.Key {
				return a.Key < b.Key
			}
			if a.Rank != b.Rank {
				return a.Rank < b.Rank
			}
			return lessValue(a.Value, b.Value)
		})
	}
}

// intArg reads an integral tuple argument; anything else is 0.
func intArg(t datalog.Tuple, key string) int {
	switch v := t.Args[key].(type) {
	case int64:
		return int(v)
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

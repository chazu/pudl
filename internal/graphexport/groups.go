package graphexport

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/resourcegroup"
)

// builtResource is a resource with the reference values that name it and
// the origin of the entry it came from.
type builtResource struct {
	r      Resource
	refs   []string
	origin string
}

// loadGroups reads which of the built resources represent one real resource.
// A failure or a conflict is a warning: resources then stay ungrouped.
func loadGroups(ctx context.Context, in Input, items []builtResource, scope datalog.TemporalScope, warn func(string)) *resourcegroup.Index {
	schemaOf := make(map[string]string, len(items))
	for _, it := range items {
		schemaOf[it.r.ID] = it.r.Schema
	}
	idx, err := resourcegroup.Load(ctx, resourcegroup.Input{
		DB: in.DB, Rules: in.Rules, Scope: scope, SchemaOf: schemaOf,
		Metadata: resourcegroup.MetadataFunc(in.Metadata),
	})
	if err != nil {
		warn(fmt.Sprintf("same_resource unavailable (%v); resources are not grouped", err))
		return nil
	}
	for _, c := range idx.Conflicts {
		var parts []string
		for ns, ids := range c.Namespaces {
			parts = append(parts, fmt.Sprintf("%d records of %s", len(ids), ns))
		}
		sort.Strings(parts)
		warn(fmt.Sprintf("resources %s were not grouped: %s share a %s key (see pudl doctor)",
			strings.Join(c.Members, ", "), strings.Join(parts, " and "), strings.Join(c.Kinds, "/")))
	}
	return idx
}

// mergeGroups replaces each group's members with one resource and returns
// which member IDs now name which group. The merged resource takes its
// labels from the member whose schema sorts first, fills an empty account,
// region or zone from the others, and carries every member's evidence and
// reference values.
func mergeGroups(items []builtResource, idx *resourcegroup.Index) ([]builtResource, map[string]string) {
	aliases := map[string]string{}
	if idx == nil || len(idx.Groups) == 0 {
		return items, aliases
	}
	members := map[string][]builtResource{}
	var out []builtResource
	for _, it := range items {
		if g, ok := idx.GroupOf(it.r.ID); ok {
			members[g.ID] = append(members[g.ID], it)
			continue
		}
		out = append(out, it)
	}
	for groupID, ms := range members {
		if len(ms) < 2 {
			out = append(out, ms...)
			continue
		}
		sort.Slice(ms, func(i, j int) bool {
			if ms[i].r.Schema != ms[j].r.Schema {
				return ms[i].r.Schema < ms[j].r.Schema
			}
			return ms[i].r.ID < ms[j].r.ID
		})
		merged := ms[0]
		merged.r.ID = groupID
		merged.r.Evidence = nil
		merged.refs = nil
		for _, m := range ms {
			aliases[m.r.ID] = groupID
			merged.r.Representations = append(merged.r.Representations, Representation{
				ID: m.r.ID, Schema: m.r.Schema, Type: m.r.Type, EntryID: m.r.EntryID, Origin: m.origin,
			})
			for _, ev := range m.r.Evidence {
				if !containsEvidence(merged.r.Evidence, ev) {
					merged.r.Evidence = append(merged.r.Evidence, ev)
				}
			}
			merged.refs = append(merged.refs, m.refs...)
			fillEmpty(&merged.r.Account, m.r.Account)
			fillEmpty(&merged.r.Region, m.r.Region)
			fillEmpty(&merged.r.Zone, m.r.Zone)
		}
		out = append(out, merged)
	}
	return out, aliases
}

func fillEmpty(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}

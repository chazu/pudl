// Package resourcegroup loads which cataloged resources represent one real
// resource: it evaluates the same_resource relation over the catalog and
// groups the links with sameresource.Build. Graph export, `pudl show` and
// `pudl doctor` read groups through it. See
// docs/design/2026-11-05-shared-identity-across-schemas.md.
package resourcegroup

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/sameresource"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// MetadataFunc looks up a schema's metadata.
type MetadataFunc func(schema string) (validator.SchemaMetadata, bool)

// Input is what Load reads.
type Input struct {
	DB    *database.CatalogDB
	Rules []datalog.Rule
	Scope datalog.TemporalScope
	// SchemaOf maps each resource that may be grouped to its current entry's
	// schema. Links naming any other resource are ignored, so groups only
	// hold resources the caller shows.
	SchemaOf map[string]string
	Metadata MetadataFunc
}

// Index is the grouping of the catalog's resources.
type Index struct {
	Groups    []sameresource.Group
	Conflicts []sameresource.Conflict
	byMember  map[string]int
}

// Load evaluates same_resource and groups the links. Two members of one
// identity namespace in a component make it a conflict, not a group.
func Load(ctx context.Context, in Input) (*Index, error) {
	tuples, err := datalog.EvaluateContext(ctx, in.DB, in.Rules, sameresource.LinkRelation, nil, in.Scope, datalog.EvalOptions{})
	if err != nil {
		return nil, fmt.Errorf("evaluate %s: %w", sameresource.LinkRelation, err)
	}
	var links []sameresource.Link
	for _, t := range tuples {
		a, okA := t.Args["a"].(string)
		b, okB := t.Args["b"].(string)
		kind, _ := t.Args["kind"].(string)
		if !okA || !okB || a == b {
			continue
		}
		if _, ok := in.SchemaOf[a]; !ok {
			continue
		}
		if _, ok := in.SchemaOf[b]; !ok {
			continue
		}
		links = append(links, sameresource.Link{A: a, B: b, Kind: kind})
	}
	namespaceOf := func(id string) string { return Namespace(in.SchemaOf[id], in.Metadata) }
	return FromLinks(links, namespaceOf), nil
}

// FromLinks builds an index from links already read.
func FromLinks(links []sameresource.Link, namespaceOf func(string) string) *Index {
	groups, conflicts := sameresource.Build(links, namespaceOf)
	x := &Index{Groups: groups, Conflicts: conflicts, byMember: map[string]int{}}
	for i, g := range groups {
		for _, m := range g.Members {
			x.byMember[m] = i
		}
	}
	return x
}

// GroupOf returns the group holding resourceID, if any.
func (x *Index) GroupOf(resourceID string) (sameresource.Group, bool) {
	if x == nil {
		return sameresource.Group{}, false
	}
	i, ok := x.byMember[resourceID]
	if !ok {
		return sameresource.Group{}, false
	}
	return x.Groups[i], true
}

// Namespace returns schema's identity namespace: the root of its base_schema
// chain, which namespaces resource IDs (see identity.ComputeResourceID). An
// empty schema has none.
func Namespace(schema string, meta MetadataFunc) string {
	if schema == "" {
		return ""
	}
	cur := schemaname.Normalize(schema)
	seen := map[string]bool{}
	for !seen[cur] {
		seen[cur] = true
		if meta == nil {
			return cur
		}
		m, ok := meta(cur)
		if !ok || m.BaseSchema == "" {
			return cur
		}
		cur = schemaname.Normalize(m.BaseSchema)
	}
	return cur
}

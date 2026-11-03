// Package graphvocab assembles what pudl ui needs to know about types and
// relationships, from CUE only:
//
//   - each schema's `_pudl.graph` block: a resource type's display info
//     (code, label, class) and its roles in the views;
//   - ground facts in rule files: graph_edge_category, graph_edge_type,
//     graph_provider, graph_account_type (which type a provider's `account`
//     attribute names) and graph_resource_type (display info for types that
//     have no schema, such as pudl's external.cidr).
//
// The vocabulary travels with every graph export, so the UI holds no type
// knowledge of its own. Build also reports problems: undeclared edge types,
// unknown categories, conflicting display info. Pure: no database access.
package graphvocab

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/validator"
)

// Vocabulary relations, declared as ground facts in rule files.
const (
	CategoryRelation     = "graph_edge_category"
	EdgeTypeRelation     = "graph_edge_type"
	ProviderRelation     = "graph_provider"
	ResourceTypeRelation = "graph_resource_type"
	AccountTypeRelation  = "graph_account_type"
	// EdgeRelation is the relation graph rules derive (see graphexport).
	EdgeRelation = "graph_edge"
)

// Category is a relationship category. The UI styles each category id.
type Category struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Meaning string `json:"meaning"`
	Order   int    `json:"order"`
	Source  string `json:"source"`
}

// EdgeType is one relationship type.
type EdgeType struct {
	Type     string `json:"type"`
	Category string `json:"category"`
	Label    string `json:"label"`
	Meaning  string `json:"meaning"`
	Source   string `json:"source"`
}

// Provider labels a provider (the first segment of a resource type).
type Provider struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Source   string `json:"source"`
}

// Admin and Roles are graphspec's, re-exported for callers of this package.
type (
	Admin = graphspec.Admin
	Roles = graphspec.Roles
)

// ResourceType is one resource type's display info and view roles. Schema
// names the schema it was read from; Source names the fact, for a type
// declared by graph_resource_type.
type ResourceType struct {
	Type   string `json:"type"`
	Code   string `json:"code"`
	Label  string `json:"label"`
	Class  string `json:"class"`
	Schema string `json:"schema,omitempty"`
	Source string `json:"source,omitempty"`
	graphspec.Roles
}

// Vocabulary is the merged vocabulary.
type Vocabulary struct {
	Categories    []Category     `json:"categories"`
	EdgeTypes     []EdgeType     `json:"edgeTypes"`
	Providers     []Provider     `json:"providers"`
	ResourceTypes []ResourceType `json:"resourceTypes"`
	Problems      []string       `json:"problems"`
}

// MetadataFunc looks up a schema's metadata.
type MetadataFunc func(schema string) (validator.SchemaMetadata, bool)

// Input is everything Build reads.
type Input struct {
	// Schemas are the loaded schema names; Metadata resolves each.
	Schemas  []string
	Metadata MetadataFunc
	Rules    []datalog.Rule
	// Skip lists resource types that are not drawn (pudl's own records).
	Skip map[string]bool
}

// Build merges the vocabulary and reports its problems.
func Build(in Input) *Vocabulary {
	v := &Vocabulary{}
	f := readFacts(in.Rules)
	v.Problems = append(v.Problems, f.problems...)
	v.Categories, v.EdgeTypes, v.Providers = f.categories, f.edgeTypes, f.providers

	byType := map[string]ResourceType{}
	schemas := append([]string(nil), in.Schemas...)
	sort.Strings(schemas)
	for _, schema := range schemas {
		m, ok := in.Metadata(schema)
		if !ok || m.ResourceType == "" || in.Skip[m.ResourceType] {
			continue
		}
		spec := SpecFor(schema, in.Metadata)
		if spec == nil || (spec.Display == nil && spec.Roles == (graphspec.Roles{})) {
			continue
		}
		rt := ResourceType{Type: m.ResourceType, Schema: schema, Roles: spec.Roles}
		if spec.Display != nil {
			rt.Code, rt.Label, rt.Class = spec.Display.Code, spec.Display.Label, spec.Display.Class
		}
		if prev, dup := byType[rt.Type]; dup {
			if !sameResourceType(prev, rt) {
				v.Problems = append(v.Problems, "resource type "+rt.Type+": "+prev.Schema+" and "+schema+" declare different graph info; using "+prev.Schema)
			}
			continue
		}
		byType[rt.Type] = rt
	}
	for _, rt := range f.resourceTypes {
		if prev, dup := byType[rt.Type]; dup {
			v.Problems = append(v.Problems, "resource type "+rt.Type+": "+rt.Source+" is ignored; schema "+prev.Schema+" declares it")
			continue
		}
		byType[rt.Type] = rt
	}
	for _, at := range f.accountTypes {
		rt, ok := byType[at.typ]
		if !ok {
			v.Problems = append(v.Problems, fmt.Sprintf("%s: account type %s has no display info; declare it in a schema's graph block or with %s", at.source, at.typ, ResourceTypeRelation))
			continue
		}
		admin := Admin{Owner: true}
		if rt.Admin != nil {
			admin.Parent = rt.Admin.Parent
		}
		rt.Admin = &admin
		byType[at.typ] = rt
	}
	for _, rt := range byType {
		v.ResourceTypes = append(v.ResourceTypes, rt)
	}
	sort.Slice(v.ResourceTypes, func(i, j int) bool { return v.ResourceTypes[i].Type < v.ResourceTypes[j].Type })

	v.Problems = append(v.Problems, crossCheck(v, in.Rules)...)
	if v.Problems == nil {
		v.Problems = []string{}
	}
	ensureLists(v)
	return v
}

// SpecFor returns the nearest `_pudl.graph` along the base_schema chain.
func SpecFor(schema string, meta MetadataFunc) *graphspec.Spec {
	seen := map[string]bool{}
	for name := schema; name != "" && !seen[name]; {
		seen[name] = true
		m, ok := meta(name)
		if !ok {
			return nil
		}
		if m.Graph != nil {
			return m.Graph
		}
		name = m.BaseSchema
	}
	return nil
}

// MissingDisplay lists the given resource types (e.g. the ones in the
// catalog) that have no display info, sorted.
func (v *Vocabulary) MissingDisplay(types []string) []string {
	known := map[string]bool{}
	for _, rt := range v.ResourceTypes {
		if rt.Code != "" {
			known[rt.Type] = true
		}
	}
	var out []string
	for _, t := range types {
		if t != "" && !known[t] {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// NoteMissingDisplay adds a problem for each of the given resource types
// (the ones the catalog holds) that the vocabulary does not know at all. A
// type with roles but no display is already reported by Build.
func (v *Vocabulary) NoteMissingDisplay(types []string) {
	declared := map[string]bool{}
	for _, rt := range v.ResourceTypes {
		declared[rt.Type] = true
	}
	for _, t := range v.MissingDisplay(types) {
		if !declared[t] {
			v.Problems = append(v.Problems, fmt.Sprintf("resource type %s is in the catalog but has no display info; pudl ui draws it as unknown (add display to its schema's _pudl graph block)", t))
		}
	}
}

// EdgeTypesIn lists the edge types in a category, sorted.
func (v *Vocabulary) EdgeTypesIn(category string) []string {
	var out []string
	for _, e := range v.EdgeTypes {
		if e.Category == category {
			out = append(out, e.Type)
		}
	}
	return out
}

// ProviderOf returns a resource type's provider: its first segment.
func ProviderOf(resourceType string) string {
	return strings.SplitN(resourceType, ".", 2)[0]
}

func sameResourceType(a, b ResourceType) bool {
	a.Schema, b.Schema = "", ""
	return a.Code == b.Code && a.Label == b.Label && a.Class == b.Class && rolesKey(a.Roles) == rolesKey(b.Roles)
}

func ensureLists(v *Vocabulary) {
	if v.Categories == nil {
		v.Categories = []Category{}
	}
	if v.EdgeTypes == nil {
		v.EdgeTypes = []EdgeType{}
	}
	if v.Providers == nil {
		v.Providers = []Provider{}
	}
	if v.ResourceTypes == nil {
		v.ResourceTypes = []ResourceType{}
	}
}

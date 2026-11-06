// Package graphexport builds the canonical resource graph the web explorer
// reads (web/src/model/canonical.ts): resources from the catalog's current
// entries, relationships from the `graph_edge` Datalog relation.
//
// Resources are labelled by each schema's `_pudl.graph` block (see the
// graphspec package). Relationships come from rules with the head
//
//	graph_edge(type, source, target, basis)
//
// where source and target are a resource ID or a reference value (an AWS ID,
// a GCP path) that resolves through identity values and `_pudl.graph.refs`.
// A reference that resolves to nothing is kept as `ref:<value>`, so the graph
// shows an uncollected resource as unknown rather than dropping the edge.
package graphexport

import "github.com/chazu/pudl/internal/graphvocab"

// EdgeRelation is the reserved relation that graph rules derive.
const EdgeRelation = graphvocab.EdgeRelation

// PropertyRelation optionally attaches details to edges:
// graph_edge_property(type, source, target, key, value). Values for one key
// collect into a list (a security group allowing ports 80 and 443 from the
// same range is one edge with port [80, 443]).
const PropertyRelation = "graph_edge_property"

// RefPrefix marks a relationship endpoint that names no cataloged resource.
const RefPrefix = "ref:"

// InternalTypes are pudl's own bookkeeping records (check models, observation
// results). They are cataloged as resources but are not infrastructure, so
// the graph leaves them out.
var InternalTypes = map[string]bool{"system_model": true, "pudl.observe": true, "pudl.observe_snapshot": true}

// Bases are the evidence bases the graph understands.
var Bases = map[string]bool{"observed": true, "declared": true, "inferred": true, "unknown": true}

// Graph is the export document.
type Graph struct {
	Meta          Meta           `json:"meta"`
	Resources     []Resource     `json:"resources"`
	Relationships []Relationship `json:"relationships"`
	// Vocabulary is what the types and relationship types mean, read from
	// CUE (see graphvocab); the UI holds no type knowledge of its own.
	Vocabulary *graphvocab.Vocabulary `json:"vocabulary,omitempty"`
}

// Meta describes the export.
type Meta struct {
	Origin      string `json:"origin"`
	Label       string `json:"label"`
	GeneratedAt string `json:"generatedAt"`
	Workspace   string `json:"workspace,omitempty"`
	// Version is the catalog change stamp the graph was built at, when the
	// producer knows it (pudl ui); clients compare it to /api/version.
	Version string `json:"version,omitempty"`
	// AsOf is set when the graph shows the catalog as recorded at a past
	// time (RFC 3339) rather than now.
	AsOf string `json:"asOf,omitempty"`
	// Partial marks a neighbourhood (Subgraph): only some resources are
	// present. Degree is each present resource's total relationship count, so
	// a client can tell how many relationships are not loaded; Totals is the
	// size of the whole graph.
	Partial bool `json:"partial,omitempty"`
	// Coverage explains what the graph leaves out of the catalog.
	Coverage *Coverage      `json:"coverage,omitempty"`
	Degree   map[string]int `json:"degree,omitempty"`
	// Owned is, for each owner resource present (account, project), how
	// many resources it owns in the whole graph (see OwnedIDs). Partial
	// graphs only.
	Owned    map[string]int `json:"owned,omitempty"`
	Totals   *Totals        `json:"totals,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// Coverage compares the graph with the catalog, so a sparse graph explains
// itself: which schemas' entries are not shown, and why.
type Coverage struct {
	// Entries counts catalog entries (collection wrappers excluded).
	Entries int `json:"entries"`
	// Resources counts resources in the graph from the catalog (externals excluded).
	Resources int `json:"resources"`
	// Hidden lists schemas with entries that are not in the graph.
	Hidden []HiddenSchema `json:"hidden,omitempty"`
	// EdgeRules counts loaded graph_edge rules; zero means no relationships.
	EdgeRules int `json:"edgeRules"`
}

// HiddenSchema is one schema's entries that the graph does not show.
type HiddenSchema struct {
	Schema  string `json:"schema"`
	Entries int    `json:"entries"`
	// Reason is "no-identity" (the schema has no identity fields, or they did
	// not resolve, as with generic catch-all types) or "internal" (pudl's own
	// records).
	Reason string `json:"reason"`
}

// Totals is the size of a whole graph.
type Totals struct {
	Resources     int `json:"resources"`
	Relationships int `json:"relationships"`
}

// Resource is one cataloged resource in its current state.
type Resource struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Provider   string         `json:"provider"`
	NativeID   string         `json:"nativeId,omitempty"`
	Account    string         `json:"account,omitempty"`
	Region     string         `json:"region,omitempty"`
	Zone       string         `json:"zone,omitempty"`
	Schema     string         `json:"schema"`
	EntryID    string         `json:"entryId"`
	Properties map[string]any `json:"properties"`
	Evidence   []Evidence     `json:"evidence"`
	// Representations lists each cataloged record of a resource that several
	// schemas describe (see same_resource). ID is then the group's ID and
	// the labels come from the first representation. Absent for a resource
	// with one record.
	Representations []Representation `json:"representations,omitempty"`
}

// Representation is one cataloged record of a grouped resource.
type Representation struct {
	ID      string `json:"id"`
	Schema  string `json:"schema"`
	Type    string `json:"type"`
	EntryID string `json:"entryId"`
	Origin  string `json:"origin"`
}

// Relationship is one derived, directed, typed edge.
type Relationship struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Source     string         `json:"source"`
	Target     string         `json:"target"`
	Basis      string         `json:"basis"`
	Properties map[string]any `json:"properties,omitempty"`
	Evidence   []Evidence     `json:"evidence"`
}

// Evidence is where a resource or relationship came from.
type Evidence struct {
	Basis      string `json:"basis"`
	Source     string `json:"source"`
	ObservedAt string `json:"observedAt,omitempty"`
	SnapshotID string `json:"snapshotId,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

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

// EdgeRelation is the reserved relation that graph rules derive.
const EdgeRelation = "graph_edge"

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
}

// Meta describes the export.
type Meta struct {
	Origin      string   `json:"origin"`
	Label       string   `json:"label"`
	GeneratedAt string   `json:"generatedAt"`
	Workspace   string   `json:"workspace,omitempty"`
	// Version is the catalog change stamp the graph was built at, when the
	// producer knows it (pudl ui); clients compare it to /api/version.
	Version string `json:"version,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
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
}

// Relationship is one derived, directed, typed edge.
type Relationship struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Source   string     `json:"source"`
	Target   string     `json:"target"`
	Basis    string     `json:"basis"`
	Evidence []Evidence `json:"evidence"`
}

// Evidence is where a resource or relationship came from.
type Evidence struct {
	Basis      string `json:"basis"`
	Source     string `json:"source"`
	ObservedAt string `json:"observedAt,omitempty"`
	SnapshotID string `json:"snapshotId,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

package graphexport

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
)

// Input is everything Build reads.
type Input struct {
	DB        *database.CatalogDB
	Rules     []datalog.Rule
	Metadata  MetadataFunc
	Load      LoadFunc
	Workspace string
	Now       time.Time
	// AsOf, when set, builds the graph as pudl had recorded it at that time:
	// each resource's latest entry by then, snapshots taken by then, and
	// facts as believed then about then. Rules and schemas are today's.
	AsOf *time.Time
}

// Build exports the current resource graph.
func Build(ctx context.Context, in Input) (*Graph, error) {
	g := &Graph{
		Meta: Meta{
			Origin:      "pudl",
			Label:       "pudl catalog",
			GeneratedAt: in.Now.UTC().Format(time.RFC3339),
			Workspace:   in.Workspace,
		},
		Resources:     []Resource{},
		Relationships: []Relationship{},
	}
	if in.Workspace != "" {
		g.Meta.Label = "pudl catalog · " + in.Workspace
	}
	warn := func(s string) { g.Meta.Warnings = append(g.Meta.Warnings, s) }

	var entries []database.CatalogEntry
	var err error
	scope := datalog.TemporalScope{}
	observations := in.DB.ResourceObservations
	if in.AsOf != nil {
		at := in.AsOf.UTC()
		g.Meta.AsOf = at.Format(time.RFC3339)
		unix := at.Unix()
		scope = datalog.TemporalScope{ValidAt: &unix, TxAt: &unix}
		entries, err = in.DB.ResourceEntriesAsOf(at)
		observations = observationsUntil(in.DB.ResourceObservations, at)
	} else {
		entries, err = in.DB.CurrentResourceEntries()
	}
	if err != nil {
		return nil, err
	}
	index := newRefIndex()
	for _, entry := range entries {
		if m, ok := in.Metadata(entry.Schema); ok && InternalTypes[m.ResourceType] {
			continue
		}
		r, refs := buildResource(ctx, entry, in.Metadata, in.Load, observations, warn)
		g.Resources = append(g.Resources, r)
		index.add(r.ID, refs)
	}
	sort.Slice(g.Resources, func(i, j int) bool { return g.Resources[i].ID < g.Resources[j].ID })

	derived, err := EvaluateEdgeRules(ctx, in.DB, in.Rules, scope, warn)
	if err != nil {
		return nil, err
	}
	if len(derived) == 0 {
		warn("no graph_edge rules are loaded; the graph has resources but no relationships")
	}
	g.Relationships = buildRelationships(derived, index, warn)
	props, err := evaluateRelation(ctx, in.DB, in.Rules, PropertyRelation, scope, warn)
	if err != nil {
		return nil, err
	}
	applyProperties(g.Relationships, props, index, warn)
	g.Meta.Coverage = coverage(in, len(g.Resources), warn)
	g.Resources = append(g.Resources, index.externalResources()...)
	return g, nil
}

// EvaluateEdgeRules evaluates each graph_edge rule on its own, so every
// relationship can name the rule that derived it. A rule that does not load
// or evaluate is reported and skipped; the rest of the graph still exports.
func EvaluateEdgeRules(ctx context.Context, db *database.CatalogDB, rules []datalog.Rule, scope datalog.TemporalScope, warn func(string)) ([]RuleTuples, error) {
	return evaluateRelation(ctx, db, rules, EdgeRelation, scope, warn)
}

// evaluateRelation evaluates each rule deriving relation on its own (with
// every other rule available), so each row can name its rule.
func evaluateRelation(ctx context.Context, db *database.CatalogDB, rules []datalog.Rule, relation string, scope datalog.TemporalScope, warn func(string)) ([]RuleTuples, error) {
	var others, edges []datalog.Rule
	for _, r := range rules {
		if r.Head.Rel == relation {
			edges = append(edges, r)
		} else {
			others = append(others, r)
		}
	}
	out := make([]RuleTuples, 0, len(edges))
	for _, r := range edges {
		if !r.Valid() {
			warn(fmt.Sprintf("rule %s does not load: %v", r.Describe(), r.LoadErr))
			continue
		}
		set := append(append([]datalog.Rule{}, others...), r)
		tuples, err := datalog.EvaluateContext(ctx, db, set, relation, nil, scope, datalog.EvalOptions{})
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			warn(fmt.Sprintf("rule %s: %v", r.Describe(), err))
			continue
		}
		out = append(out, RuleTuples{Rule: r, Tuples: tuples})
	}
	return out, nil
}

// observationsUntil keeps only snapshots taken at or before t.
func observationsUntil(list ObservationsFunc, t time.Time) ObservationsFunc {
	return func(ctx context.Context, resourceID string) ([]database.ResourceObservation, error) {
		all, err := list(ctx, resourceID)
		out := all[:0:0]
		for _, o := range all {
			if !o.ObservedAt.After(t) {
				out = append(out, o)
			}
		}
		return out, err
	}
}

// coverage compares the graph with the catalog. Counts are for the catalog
// as it is now, also for an as-of graph.
func coverage(in Input, resources int, warn func(string)) *Coverage {
	c := &Coverage{Resources: resources}
	for _, r := range in.Rules {
		if r.Head.Rel == EdgeRelation && r.Valid() {
			c.EdgeRules++
		}
	}
	counts, err := in.DB.SchemaCoverageCounts()
	if err != nil {
		warn(fmt.Sprintf("catalog coverage unavailable: %v", err))
		return c
	}
	for _, s := range counts {
		c.Entries += s.Entries
		m, ok := in.Metadata(s.Schema)
		switch {
		case ok && InternalTypes[m.ResourceType]:
			c.Hidden = append(c.Hidden, HiddenSchema{Schema: s.Schema, Entries: s.Entries, Reason: "internal"})
		case s.Unidentified > 0:
			c.Hidden = append(c.Hidden, HiddenSchema{Schema: s.Schema, Entries: s.Unidentified, Reason: "no-identity"})
		}
	}
	sort.SliceStable(c.Hidden, func(i, j int) bool { return c.Hidden[i].Entries > c.Hidden[j].Entries })
	return c
}

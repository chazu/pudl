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

	entries, err := in.DB.CurrentResourceEntries()
	if err != nil {
		return nil, err
	}
	index := newRefIndex()
	for _, entry := range entries {
		if m, ok := in.Metadata(entry.Schema); ok && InternalTypes[m.ResourceType] {
			continue
		}
		r, refs := buildResource(ctx, entry, in.Metadata, in.Load, in.DB.ResourceObservations, warn)
		g.Resources = append(g.Resources, r)
		index.add(r.ID, refs)
	}
	sort.Slice(g.Resources, func(i, j int) bool { return g.Resources[i].ID < g.Resources[j].ID })

	derived, err := EvaluateEdgeRules(ctx, in.DB, in.Rules, warn)
	if err != nil {
		return nil, err
	}
	if len(derived) == 0 {
		warn("no graph_edge rules are loaded; the graph has resources but no relationships")
	}
	g.Relationships = buildRelationships(derived, index, warn)
	return g, nil
}

// EvaluateEdgeRules evaluates each graph_edge rule on its own, so every
// relationship can name the rule that derived it. A rule that does not load
// or evaluate is reported and skipped; the rest of the graph still exports.
func EvaluateEdgeRules(ctx context.Context, db *database.CatalogDB, rules []datalog.Rule, warn func(string)) ([]RuleTuples, error) {
	var others, edges []datalog.Rule
	for _, r := range rules {
		if r.Head.Rel == EdgeRelation {
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
		tuples, err := datalog.EvaluateContext(ctx, db, set, EdgeRelation, nil, datalog.TemporalScope{}, datalog.EvalOptions{})
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

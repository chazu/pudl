package acute

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// RunSetModel is one explicitly selected model plus any registry aliases that
// authored depends_on/source names may use. Name always comes from Template.
type RunSetModel struct {
	Template *systemmodel.ModelTemplate
	Aliases  []string
}

type RunSetEdge struct {
	From    string   `json:"from"` // consumer
	To      string   `json:"to"`   // producer/prerequisite
	Sources []string `json:"sources"`
}

type runSetEdgeKey struct{ from, to string }

// RunSetPlan is a deterministic, exact named set. Ordered is producer-first;
// Edges retain the canonical consumer-to-producer relation direction.
type RunSetPlan struct {
	Models  map[string]RunSetModel
	Edges   []RunSetEdge
	Ordered []string
}

func NewRunSetPlan(selected []RunSetModel) (*RunSetPlan, error) {
	if len(selected) == 0 {
		return nil, fmt.Errorf("run set requires at least one model")
	}
	models := make(map[string]RunSetModel, len(selected))
	aliases := map[string]string{}
	for _, member := range selected {
		if member.Template == nil || member.Template.Name == "" {
			return nil, fmt.Errorf("run set contains an invalid model template")
		}
		name := member.Template.Name
		if _, duplicate := models[name]; duplicate {
			return nil, fmt.Errorf("run set contains duplicate model %q", name)
		}
		models[name] = member
		for _, alias := range append([]string{name}, member.Aliases...) {
			if owner, exists := aliases[alias]; exists && owner != name {
				return nil, fmt.Errorf("run set alias %q is ambiguous between %q and %q", alias, owner, name)
			}
			aliases[alias] = name
		}
	}

	edges := map[runSetEdgeKey]map[string]struct{}{}
	addEdge := func(from, rawTo, source string, required bool) error {
		to, exists := aliases[rawTo]
		if !exists {
			if required {
				return fmt.Errorf("model %q binding producer %q is outside the explicit run set", from, rawTo)
			}
			return nil
		}
		if from == to {
			return fmt.Errorf("model %q has a self-dependency", from)
		}
		key := runSetEdgeKey{from: from, to: to}
		if edges[key] == nil {
			edges[key] = map[string]struct{}{}
		}
		edges[key][source] = struct{}{}
		return nil
	}
	for name, member := range models {
		for _, producer := range member.Template.BindingProducers {
			if err := addEdge(name, producer, "binding", true); err != nil {
				return nil, err
			}
		}
		for _, dependency := range member.Template.DependsOn {
			if dependency == name {
				return nil, fmt.Errorf("model %q has a self-dependency", name)
			}
			if err := addEdge(name, dependency, "declared", false); err != nil {
				return nil, err
			}
		}
	}

	ordered, err := topologicalRunSetOrder(models, edges)
	if err != nil {
		return nil, err
	}
	planEdges := make([]RunSetEdge, 0, len(edges))
	for key, sourceSet := range edges {
		sources := make([]string, 0, len(sourceSet))
		for source := range sourceSet {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		planEdges = append(planEdges, RunSetEdge{From: key.from, To: key.to, Sources: sources})
	}
	sort.Slice(planEdges, func(i, j int) bool {
		if planEdges[i].From != planEdges[j].From {
			return planEdges[i].From < planEdges[j].From
		}
		return planEdges[i].To < planEdges[j].To
	})
	return &RunSetPlan{Models: models, Edges: planEdges, Ordered: ordered}, nil
}

func topologicalRunSetOrder(models map[string]RunSetModel, edges map[runSetEdgeKey]map[string]struct{}) ([]string, error) {
	prerequisites := make(map[string]map[string]struct{}, len(models))
	dependents := make(map[string][]string, len(models))
	for name := range models {
		prerequisites[name] = map[string]struct{}{}
	}
	for edge := range edges {
		prerequisites[edge.from][edge.to] = struct{}{}
		dependents[edge.to] = append(dependents[edge.to], edge.from)
	}
	var ready []string
	for name, required := range prerequisites {
		if len(required) == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)
	ordered := make([]string, 0, len(models))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		ordered = append(ordered, name)
		sort.Strings(dependents[name])
		for _, dependent := range dependents[name] {
			delete(prerequisites[dependent], name)
			if len(prerequisites[dependent]) == 0 {
				ready = append(ready, dependent)
				sort.Strings(ready)
			}
		}
	}
	if len(ordered) != len(models) {
		var cycle []string
		for name, required := range prerequisites {
			if len(required) > 0 {
				cycle = append(cycle, name)
			}
		}
		sort.Strings(cycle)
		return nil, fmt.Errorf("run set dependency cycle among: %s", strings.Join(cycle, ", "))
	}
	return ordered, nil
}

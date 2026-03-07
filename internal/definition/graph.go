package definition

import (
	"fmt"
	"sort"
)

// Graph represents the dependency graph between definitions based on socket wiring.
// An edge from A to B means "A depends on B" (B must be processed first).
type Graph struct {
	nodes   map[string]bool            // set of definition names
	deps    map[string]map[string]bool // node -> set of nodes it depends on
	rdeps   map[string]map[string]bool // node -> set of nodes that depend on it
}

// BuildGraph constructs a dependency graph from definitions.
// Edges are derived from socket bindings: if definition A references
// definition B's outputs, then A depends on B.
func BuildGraph(definitions []DefinitionInfo) *Graph {
	g := &Graph{
		nodes: make(map[string]bool),
		deps:  make(map[string]map[string]bool),
		rdeps: make(map[string]map[string]bool),
	}

	// Register all definitions as nodes
	for _, def := range definitions {
		g.nodes[def.Name] = true
	}

	// Build edges from socket bindings
	for _, def := range definitions {
		for _, binding := range def.SocketBindings {
			refName := extractReferencedDef(binding)
			if refName != "" && g.nodes[refName] && refName != def.Name {
				g.addDep(def.Name, refName)
			}
		}
	}

	return g
}

// addDep records that `from` depends on `dep`.
func (g *Graph) addDep(from, dep string) {
	if g.deps[from] == nil {
		g.deps[from] = make(map[string]bool)
	}
	if g.rdeps[dep] == nil {
		g.rdeps[dep] = make(map[string]bool)
	}
	g.deps[from][dep] = true
	g.rdeps[dep][from] = true
}

// TopologicalSort returns definitions in dependency order using Kahn's algorithm.
// Nodes with no dependencies come first. Returns an error if a cycle is detected.
func (g *Graph) TopologicalSort() ([]string, error) {
	if len(g.nodes) == 0 {
		return nil, nil
	}

	// Calculate in-degree (number of dependencies) for each node
	inDegree := make(map[string]int)
	for node := range g.nodes {
		inDegree[node] = len(g.deps[node])
	}

	// Start with nodes that have no dependencies
	var queue []string
	for node := range g.nodes {
		if inDegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	sort.Strings(queue)

	var result []string

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// For each node that depends on the processed node,
		// decrement its in-degree
		dependents := g.rdeps[node]
		for dep := range dependents {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				// Insert in sorted order for determinism
				inserted := false
				for i, q := range queue {
					if dep < q {
						queue = append(queue[:i+1], queue[i:]...)
						queue[i] = dep
						inserted = true
						break
					}
				}
				if !inserted {
					queue = append(queue, dep)
				}
			}
		}
	}

	if len(result) != len(g.nodes) {
		return nil, fmt.Errorf("cycle detected in definition dependency graph")
	}

	return result, nil
}

// GetDependencies returns the definitions that the given definition depends on.
func (g *Graph) GetDependencies(name string) []string {
	deps := g.deps[name]
	if len(deps) == 0 {
		return nil
	}

	var result []string
	for dep := range deps {
		result = append(result, dep)
	}
	sort.Strings(result)
	return result
}

// GetDependents returns the definitions that depend on the given definition.
func (g *Graph) GetDependents(name string) []string {
	rdeps := g.rdeps[name]
	if len(rdeps) == 0 {
		return nil
	}

	var result []string
	for dep := range rdeps {
		result = append(result, dep)
	}
	sort.Strings(result)
	return result
}

// extractReferencedDef extracts the definition name from a binding expression.
// e.g., "prod_vpc.outputs.vpc_id" -> "prod_vpc"
func extractReferencedDef(binding string) string {
	for i, ch := range binding {
		if ch == '.' {
			return binding[:i]
		}
	}
	return ""
}

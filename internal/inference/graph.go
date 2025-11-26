package inference

import (
	"sort"

	"pudl/internal/validator"
)

// InheritanceGraph tracks schema inheritance relationships for specificity ordering.
// Schemas that extend other schemas (via base_schema in _pudl metadata) are considered
// more specific. The graph enables traversal from most-specific to least-specific.
type InheritanceGraph struct {
	children map[string][]string // parent -> children (more specific schemas)
	parents  map[string]string   // child -> parent (less specific schema)
	priority map[string]int      // schema -> cascade_priority from metadata
	roots    []string            // schemas with no parent (base schemas)
	leaves   []string            // schemas with no children (most specific)
}

// BuildInheritanceGraph constructs an inheritance graph from schema metadata.
// It uses the base_schema field in _pudl metadata to determine parent-child relationships.
func BuildInheritanceGraph(metadata map[string]validator.SchemaMetadata) *InheritanceGraph {
	g := &InheritanceGraph{
		children: make(map[string][]string),
		parents:  make(map[string]string),
		priority: make(map[string]int),
	}

	// First pass: record all schemas and their priorities
	allSchemas := make(map[string]bool)
	for schemaName, meta := range metadata {
		allSchemas[schemaName] = true
		g.priority[schemaName] = meta.CascadePriority
	}

	// Second pass: build parent-child relationships
	for schemaName, meta := range metadata {
		if meta.BaseSchema != "" {
			// This schema has a parent
			g.parents[schemaName] = meta.BaseSchema
			g.children[meta.BaseSchema] = append(g.children[meta.BaseSchema], schemaName)
		}
	}

	// Identify roots (schemas with no parent)
	for schemaName := range allSchemas {
		if _, hasParent := g.parents[schemaName]; !hasParent {
			g.roots = append(g.roots, schemaName)
		}
	}

	// Identify leaves (schemas with no children)
	for schemaName := range allSchemas {
		if _, hasChildren := g.children[schemaName]; !hasChildren {
			g.leaves = append(g.leaves, schemaName)
		}
	}

	// Sort for deterministic ordering
	sort.Strings(g.roots)
	sort.Strings(g.leaves)

	return g
}

// GetMostSpecificFirst returns all schemas sorted by specificity (most specific first).
// Specificity is determined by:
// 1. Inheritance depth (leaves before roots)
// 2. cascade_priority (higher values first) as tiebreaker
func (g *InheritanceGraph) GetMostSpecificFirst() []string {
	// Calculate depth for each schema (distance from root)
	depths := make(map[string]int)
	for schema := range g.priority {
		depths[schema] = g.calculateDepth(schema)
	}

	// Collect all schemas
	var schemas []string
	for schema := range g.priority {
		schemas = append(schemas, schema)
	}

	// Sort by depth (descending), then by priority (descending)
	sort.Slice(schemas, func(i, j int) bool {
		depthI, depthJ := depths[schemas[i]], depths[schemas[j]]
		if depthI != depthJ {
			return depthI > depthJ // Higher depth = more specific
		}
		// Tiebreaker: cascade_priority
		return g.priority[schemas[i]] > g.priority[schemas[j]]
	})

	return schemas
}

// calculateDepth returns the inheritance depth of a schema (0 = root)
func (g *InheritanceGraph) calculateDepth(schema string) int {
	depth := 0
	current := schema
	for {
		parent, hasParent := g.parents[current]
		if !hasParent {
			break
		}
		depth++
		current = parent
		// Safety: prevent infinite loops from circular references
		if depth > 100 {
			break
		}
	}
	return depth
}

// GetCascadeChain returns the cascade chain for a schema, from most specific to least.
// This follows the inheritance chain up to the root, then adds the catchall.
func (g *InheritanceGraph) GetCascadeChain(schema string) []string {
	chain := []string{schema}

	// Walk up the inheritance tree
	current := schema
	for {
		parent, hasParent := g.parents[current]
		if !hasParent {
			break
		}
		chain = append(chain, parent)
		current = parent
		// Safety: prevent infinite loops
		if len(chain) > 100 {
			break
		}
	}

	return chain
}

// GetChildren returns the direct children (more specific schemas) of a schema
func (g *InheritanceGraph) GetChildren(schema string) []string {
	children := g.children[schema]
	if children == nil {
		return []string{}
	}
	result := make([]string, len(children))
	copy(result, children)
	return result
}

// GetParent returns the parent (less specific schema) of a schema, if any
func (g *InheritanceGraph) GetParent(schema string) (string, bool) {
	parent, exists := g.parents[schema]
	return parent, exists
}

// GetRoots returns all root schemas (those with no parent)
func (g *InheritanceGraph) GetRoots() []string {
	result := make([]string, len(g.roots))
	copy(result, g.roots)
	return result
}

// GetLeaves returns all leaf schemas (those with no children - most specific)
func (g *InheritanceGraph) GetLeaves() []string {
	result := make([]string, len(g.leaves))
	copy(result, g.leaves)
	return result
}

// GetPriority returns the cascade_priority for a schema
func (g *InheritanceGraph) GetPriority(schema string) int {
	return g.priority[schema]
}

// IsLeaf returns true if the schema has no children (is most specific in its chain)
func (g *InheritanceGraph) IsLeaf(schema string) bool {
	_, hasChildren := g.children[schema]
	return !hasChildren
}

// IsRoot returns true if the schema has no parent (is a base schema)
func (g *InheritanceGraph) IsRoot(schema string) bool {
	_, hasParent := g.parents[schema]
	return !hasParent
}

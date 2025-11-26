package inference

import (
	"testing"

	"pudl/internal/validator"
)

func TestBuildInheritanceGraph(t *testing.T) {
	metadata := map[string]validator.SchemaMetadata{
		"unknown.#CatchAll": {
			SchemaType:      "catchall",
			CascadePriority: 0,
		},
		"aws.#Resource": {
			SchemaType:      "base",
			ResourceType:    "aws.resource",
			CascadePriority: 50,
		},
		"aws.#EC2Instance": {
			SchemaType:      "base",
			ResourceType:    "aws.ec2.instance",
			BaseSchema:      "aws.#Resource",
			CascadePriority: 80,
		},
		"aws.#CompliantEC2Instance": {
			SchemaType:      "policy",
			ResourceType:    "aws.ec2.instance",
			BaseSchema:      "aws.#EC2Instance",
			CascadePriority: 90,
		},
	}

	g := BuildInheritanceGraph(metadata)

	// Test roots
	roots := g.GetRoots()
	if len(roots) != 2 {
		t.Errorf("Expected 2 roots, got %d: %v", len(roots), roots)
	}

	// Test leaves
	leaves := g.GetLeaves()
	if len(leaves) != 2 {
		t.Errorf("Expected 2 leaves, got %d: %v", len(leaves), leaves)
	}

	// Test parent relationships
	parent, hasParent := g.GetParent("aws.#CompliantEC2Instance")
	if !hasParent || parent != "aws.#EC2Instance" {
		t.Errorf("Expected aws.#EC2Instance as parent, got %s (hasParent=%v)", parent, hasParent)
	}

	parent, hasParent = g.GetParent("aws.#EC2Instance")
	if !hasParent || parent != "aws.#Resource" {
		t.Errorf("Expected aws.#Resource as parent, got %s (hasParent=%v)", parent, hasParent)
	}

	_, hasParent = g.GetParent("aws.#Resource")
	if hasParent {
		t.Error("aws.#Resource should have no parent")
	}

	// Test children relationships
	children := g.GetChildren("aws.#EC2Instance")
	if len(children) != 1 || children[0] != "aws.#CompliantEC2Instance" {
		t.Errorf("Expected [aws.#CompliantEC2Instance] as children, got %v", children)
	}
}

func TestGetMostSpecificFirst(t *testing.T) {
	metadata := map[string]validator.SchemaMetadata{
		"unknown.#CatchAll": {
			CascadePriority: 0,
		},
		"aws.#Resource": {
			CascadePriority: 50,
		},
		"aws.#EC2Instance": {
			BaseSchema:      "aws.#Resource",
			CascadePriority: 80,
		},
		"aws.#CompliantEC2Instance": {
			BaseSchema:      "aws.#EC2Instance",
			CascadePriority: 90,
		},
	}

	g := BuildInheritanceGraph(metadata)
	ordered := g.GetMostSpecificFirst()

	// CompliantEC2Instance should be first (depth 2, priority 90)
	if ordered[0] != "aws.#CompliantEC2Instance" {
		t.Errorf("Expected aws.#CompliantEC2Instance first, got %s", ordered[0])
	}

	// EC2Instance should be second (depth 1, priority 80)
	if ordered[1] != "aws.#EC2Instance" {
		t.Errorf("Expected aws.#EC2Instance second, got %s", ordered[1])
	}

	// Resource and CatchAll are both roots (depth 0), sorted by priority
	// Resource (50) should come before CatchAll (0)
	if ordered[2] != "aws.#Resource" {
		t.Errorf("Expected aws.#Resource third, got %s", ordered[2])
	}

	if ordered[3] != "unknown.#CatchAll" {
		t.Errorf("Expected unknown.#CatchAll last, got %s", ordered[3])
	}
}

func TestGetCascadeChain(t *testing.T) {
	metadata := map[string]validator.SchemaMetadata{
		"aws.#Resource": {
			CascadePriority: 50,
		},
		"aws.#EC2Instance": {
			BaseSchema:      "aws.#Resource",
			CascadePriority: 80,
		},
		"aws.#CompliantEC2Instance": {
			BaseSchema:      "aws.#EC2Instance",
			CascadePriority: 90,
		},
	}

	g := BuildInheritanceGraph(metadata)

	// Test cascade chain from most specific
	chain := g.GetCascadeChain("aws.#CompliantEC2Instance")
	expected := []string{"aws.#CompliantEC2Instance", "aws.#EC2Instance", "aws.#Resource"}

	if len(chain) != len(expected) {
		t.Fatalf("Expected chain length %d, got %d: %v", len(expected), len(chain), chain)
	}

	for i, schema := range expected {
		if chain[i] != schema {
			t.Errorf("Expected chain[%d] = %s, got %s", i, schema, chain[i])
		}
	}

	// Test cascade chain from root
	chain = g.GetCascadeChain("aws.#Resource")
	if len(chain) != 1 || chain[0] != "aws.#Resource" {
		t.Errorf("Expected [aws.#Resource], got %v", chain)
	}
}

func TestCalculateDepth(t *testing.T) {
	metadata := map[string]validator.SchemaMetadata{
		"root": {},
		"level1": {BaseSchema: "root"},
		"level2": {BaseSchema: "level1"},
		"level3": {BaseSchema: "level2"},
	}

	g := BuildInheritanceGraph(metadata)

	tests := []struct {
		schema   string
		expected int
	}{
		{"root", 0},
		{"level1", 1},
		{"level2", 2},
		{"level3", 3},
	}

	for _, tc := range tests {
		depth := g.calculateDepth(tc.schema)
		if depth != tc.expected {
			t.Errorf("Expected depth %d for %s, got %d", tc.expected, tc.schema, depth)
		}
	}
}

func TestIsLeafAndIsRoot(t *testing.T) {
	metadata := map[string]validator.SchemaMetadata{
		"root": {},
		"middle": {BaseSchema: "root"},
		"leaf": {BaseSchema: "middle"},
	}

	g := BuildInheritanceGraph(metadata)

	// Test IsRoot
	if !g.IsRoot("root") {
		t.Error("root should be a root")
	}
	if g.IsRoot("middle") {
		t.Error("middle should not be a root")
	}
	if g.IsRoot("leaf") {
		t.Error("leaf should not be a root")
	}

	// Test IsLeaf
	if g.IsLeaf("root") {
		t.Error("root should not be a leaf")
	}
	if g.IsLeaf("middle") {
		t.Error("middle should not be a leaf")
	}
	if !g.IsLeaf("leaf") {
		t.Error("leaf should be a leaf")
	}
}

func TestEmptyGraph(t *testing.T) {
	g := BuildInheritanceGraph(map[string]validator.SchemaMetadata{})

	if len(g.GetRoots()) != 0 {
		t.Error("Empty graph should have no roots")
	}
	if len(g.GetLeaves()) != 0 {
		t.Error("Empty graph should have no leaves")
	}
	if len(g.GetMostSpecificFirst()) != 0 {
		t.Error("Empty graph should return empty ordering")
	}
}

func TestPriorityTiebreaker(t *testing.T) {
	// Two schemas at the same depth, different priorities
	metadata := map[string]validator.SchemaMetadata{
		"schemaA": {CascadePriority: 100},
		"schemaB": {CascadePriority: 50},
		"schemaC": {CascadePriority: 75},
	}

	g := BuildInheritanceGraph(metadata)
	ordered := g.GetMostSpecificFirst()

	// Should be ordered by priority: A (100), C (75), B (50)
	if ordered[0] != "schemaA" {
		t.Errorf("Expected schemaA first, got %s", ordered[0])
	}
	if ordered[1] != "schemaC" {
		t.Errorf("Expected schemaC second, got %s", ordered[1])
	}
	if ordered[2] != "schemaB" {
		t.Errorf("Expected schemaB third, got %s", ordered[2])
	}
}

package component

// ComponentKind classifies a component's role in a system.
//   contract:  defines types, schemas, interfaces (what)
//   instance:  concrete implementation producing artifacts (how)
//   package:   composes other components into a cohesive unit
//   rule:      defines structural constraints and validation
#ComponentKind: "contract" | "instance" | "package" | "rule"

// Component represents a classifiable unit in a system hierarchy.
// Inspired by the BRICK pattern: each directory/module in a project
// can be classified by its role.
#Component: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "component"
		identity_fields: ["path"]
		tracked_fields: ["kind"]
	}

	path:         string
	kind:         #ComponentKind
	description?: string
	composes?: [...string]
	implements?: string

	// Only packages may compose other components
	if kind != "package" {
		composes?: []
	}

	// Only instances may implement a contract
	if kind != "instance" {
		implements?: ""
	}
}

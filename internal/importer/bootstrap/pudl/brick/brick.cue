package brick

// BRICK: Building block, Role, Implementation, Configuration, Kit.
// Five registers on every platform artifact.
//
// This schema enables pudl definitions to carry BRICK classification
// metadata that flows through export-actions to mu.json and back in
// manifests, enabling typed tracking of infrastructure components.

// BrickKind classifies a block's role in the ecosystem.
#BrickKind: "relationship" | "interface" | "component" | "kit"

// Target represents a mu target with BRICK classification metadata.
// When pudl exports a definition as a mu target, the BRICK fields
// (kind, implements, composes) travel alongside the standard mu
// fields (toolchain, config, sources).
#Target: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "brick.target"
		identity_fields: ["name"]
		tracked_fields:  ["kind", "toolchain", "config"]
	}

	name:      string       // e.g. "//k8s/api-deployment"
	kind:      #BrickKind
	toolchain: string       // mu toolchain name (e.g. "k8s", "terraform", "file")
	desc?:     string       // human-readable description

	// Interface this component satisfies (only for component kind).
	implements?: string

	// Blocks this kit composes (only for kit kind).
	composes?: [...string]

	// Source files for this target.
	sources?: [...string]

	// Plugin-specific configuration (opaque to BRICK, validated by mu plugin).
	config: {...}

	// Only component bricks may have implements.
	if kind != "component" {
		implements?: ""
	}

	// Only kit bricks may have composes.
	if kind != "kit" {
		composes?: []
	}
}

// Interface represents a contract that components implement.
// Interfaces define schemas, templates, and validation rules
// that components must satisfy.
#Interface: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "brick.interface"
		identity_fields: ["name"]
		tracked_fields:  ["contract"]
	}

	name:     string
	kind:     "interface" // always interface
	desc?:    string
	contract: {...}       // the schema/contract that components must satisfy
}

// Kit represents a composition of targets deployed together.
// Kits track which components they contain and their collective
// convergence state.
#Kit: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "brick.kit"
		identity_fields: ["name"]
		tracked_fields:  ["composes"]
	}

	name:     string
	kind:     "kit" // always kit
	desc?:    string
	composes: [...string] // target names this kit contains
}

package pulumi

// One resource of a Pulumi stack's state, from GET
// /api/stacks/<org>/<project>/<stack>/export (deployment.resources). State
// holds secrets that Pulumi does not mark as secret, so the collector's
// transform keeps only these fields: no inputs, no outputs other than the
// identity values in live, no propertyDependencies and no initErrors.
// Components (custom: false) are kept for their place in the parent chain.
#Resource: {
	_pudl: {
		schema_type:   "base"
		resource_type: "pulumi.resource"
		identity_fields: ["urn"]
		review: status: "draft"
		// A root resource has no parent: rules match has_parent: true
		// before they join on parent, which reads as null otherwise.
		facts: pulumi_resource: args: {
			urn:        "urn"
			stack:      "stack"
			type:       "type"
			custom:     "custom"
			external:   "external"
			parent:     "parent"
			has_parent: {exists: "parent"}
		}
	}
	// urn:pulumi:<stack>::<project>::<type chain>::<name>
	urn: =~"^urn:pulumi:"
	// <org>/<project>/<stack>
	stack: =~"^[^/]+/[^/]+/[^/]+$"
	type:  string
	// The provider's ID of the real resource; components have none.
	id?: string
	// false for a component, which only groups other resources.
	custom: bool
	// true for a resource Pulumi reads but does not manage (a `get`).
	external: bool
	protect?: bool
	parent?:  string
	dependencies?: [...string]
	created?:        string
	modified?:       string
	sourcePosition?: string
	// The provider resource's URN, without its ID.
	provider?: string
	// Identity values of the real resource, kept only as plain strings.
	live?: {
		// kubernetes:* types.
		metadata?: {
			uid?:       string
			name?:      string
			namespace?: string
		}
	}
}

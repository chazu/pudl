package rules

// Pulumi content pack rules. Relations come from the pack's fact projection:
// pulumi_resource (pulumi.#Resource) and pulumi_stack_source (pulumi.#Stack).

// pulumi_managed — a real resource that a stack creates and manages: not a
// component (custom: false) and not one it only reads (external: true).
pulumi_managed: {
	head: {rel: "pulumi_managed", args: {urn: "$U", stack: "$S"}}
	body: [{rel: "pulumi_resource", args: {urn: "$U", stack: "$S", custom: true, external: false}}]
}

// pulumi_ancestor — every resource above a resource in its parent chain: its
// components, up to the stack's root resource.
pulumi_ancestor_parent: {
	head: {rel: "pulumi_ancestor", args: {urn: "$U", ancestor: "$P"}}
	body: [{rel: "pulumi_resource", args: {urn: "$U", parent: "$P", has_parent: true}}]
}

pulumi_ancestor_rec: {
	head: {rel: "pulumi_ancestor", args: {urn: "$U", ancestor: "$A"}}
	body: [
		{rel: "pulumi_resource", args: {urn: "$U", parent: "$P", has_parent: true}},
		{rel: "pulumi_ancestor", args: {urn: "$P", ancestor: "$A"}},
	]
}

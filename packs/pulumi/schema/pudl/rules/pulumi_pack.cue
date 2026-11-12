package rules

// Pulumi content pack rules. Relations come from the pack's fact projection:
// pulumi_resource (pulumi.#Resource) and pulumi_stack_source (pulumi.#Stack).

// gcp.resource — a GCP resource by its relative resource name
// (projects/P/global/networks/N, projects/P/serviceAccounts/E, b/<bucket>).
// The GCP pack declares the same fact, so either pack alone passes doctor.
same_resource_kind_gcp_resource: {
	head: {rel: "same_resource_kind", args: {kind: "gcp.resource", label: "GCP resource",
		meaning: "A Google Cloud resource, keyed by its relative resource name."}}
}

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

// ---- Inspector details (graph_detail). A state resource draws only as a
// representation of its live resource; these rows give that node a Pulumi
// section: the stack and type, the repo, directory and commit of the
// stack's latest update, and the component path (rank is the depth).

pulumi_detail_stack: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "stack", value: "$S"}}
	body: [{rel: "pulumi_resource", args: {resource_id: "$R", stack: "$S"}}]
}

pulumi_detail_type: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "type", value: "$T"}}
	body: [{rel: "pulumi_resource", args: {resource_id: "$R", type: "$T"}}]
}

pulumi_detail_repo: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "repo", value: "$V"}}
	body: [
		{rel: "pulumi_resource", args: {resource_id: "$R", stack: "$S"}},
		{rel: "pulumi_stack_source", args: {stack: "$S", repo: "$V"}},
	]
}

pulumi_detail_root: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "root", value: "$V"}}
	body: [
		{rel: "pulumi_resource", args: {resource_id: "$R", stack: "$S"}},
		{rel: "pulumi_stack_source", args: {stack: "$S", root: "$V"}},
	]
}

pulumi_detail_commit: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "commit", value: "$V"}}
	body: [
		{rel: "pulumi_resource", args: {resource_id: "$R", stack: "$S"}},
		{rel: "pulumi_stack_source", args: {stack: "$S", commit: "$V"}},
	]
}

// The update ran from a working tree with uncommitted changes: the commit
// is approximate.
pulumi_detail_commit_dirty: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "commit note", value: "approximate: the update ran with uncommitted changes"}}
	body: [
		{rel: "pulumi_resource", args: {resource_id: "$R", stack: "$S"}},
		{rel: "pulumi_stack_source", args: {stack: "$S", dirty: true}},
	]
}

// One row per ancestor, the stack's root resource first (rank = depth).
pulumi_detail_component: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "component", value: "$N", rank: "$D"}}
	body: [
		{rel: "pulumi_resource", args: {resource_id: "$R", urn: "$U"}},
		{rel: "pulumi_ancestor", args: {urn: "$U", ancestor: "$A"}},
		{rel: "pulumi_resource", args: {urn: "$A", name: "$N", depth: "$D"}},
	]
}

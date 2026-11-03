package rules

// pudl's graph vocabulary: what relationship categories and edge types mean,
// read by `pudl graph export`, `pudl graph vocab` and pudl ui. Every entry is
// a fact (a rule with no body). Content packs add their own edge types,
// providers and account types the same way, in their own rule files; a
// resource type's code, label and view roles live in its schema's
// `_pudl: graph` block. See docs/schema-authoring.md, "Make a type appear in
// pudl ui".
//
// A fact written out in full:
//
//	graph_edge_type_in_subnet: {
//		head: {rel: "graph_edge_type", args: {type: "in_subnet",
//			category: "containment", label: "in subnet",
//			meaning: "Has an address in this subnet."}}
//	}
//
// This file generates the facts from tables to keep them readable.

// Categories are the fixed set pudl ui styles (colour, line pattern). An
// edge type in any other category is drawn as "other". order sorts legends.
_graphCategories: {
	network:     {label: "Network config", order: 1, meaning: "Configured connectivity. Does not establish reachability on its own."}
	traffic:     {label: "Observed traffic", order: 2, meaning: "Communication recorded by a collector during its observation window."}
	dependency:  {label: "Dependency", order: 3, meaning: "Configured or declared dependency. Not proof that traffic flows."}
	security:    {label: "Security", order: 4, meaning: "Security control or exposure rule. Permission, not observed traffic."}
	iam:         {label: "Identity & access", order: 5, meaning: "Policy attachment, binding or trust. Not evaluated effective permission."}
	containment: {label: "Containment", order: 6, meaning: "Placement or membership. Says nothing about connectivity or access."}
	other:       {label: "Other", order: 7, meaning: "Relationship type PUDL has no registered meaning for."}
}

for id, c in _graphCategories {
	"graph_edge_category_\(id)": {head: {rel: "graph_edge_category", args: {category: id, label: c.label, meaning: c.meaning, order: c.order}}}
}

// Edge types that mean the same thing for every provider. Each meaning is
// the strongest claim an edge supports; pudl ui shows it next to the edge.
_graphEdgeTypes: {
	// containment
	in_organization: {category: "containment", label: "in org", meaning: "Administrative placement under an organization."}
	in_folder:       {category: "containment", label: "in folder", meaning: "Administrative placement under a folder."}
	in_account:      {category: "containment", label: "in account", meaning: "Owned by this account."}
	in_project:      {category: "containment", label: "in project", meaning: "Owned by this project."}
	in_network:      {category: "containment", label: "in network", meaning: "Defined inside this VPC network."}
	in_subnet:       {category: "containment", label: "in subnet", meaning: "Has an address in this subnet."}
	attached_to:     {category: "containment", label: "attached to", meaning: "Attached to this resource (interface, gateway)."}
	member_of:       {category: "containment", label: "member of", meaning: "Declared member of a logical service."}
	part_of:         {category: "containment", label: "part of", meaning: "Logical service belongs to this application."}
	// network
	routes_via:       {category: "network", label: "route via", meaning: "A route table entry targets this gateway. Traffic still subject to security rules."}
	associated_with:  {category: "network", label: "associated", meaning: "Route table is associated with this subnet."}
	peers_with:       {category: "network", label: "peering", meaning: "Peering is configured. Reachability also requires routes and security rules; not evaluated."}
	endpoint_for:     {category: "network", label: "endpoint for", meaning: "Private endpoint to this service. Access still governed by IAM and endpoint policy."}
	private_ip_in:    {category: "network", label: "private IP in", meaning: "Has a private address allocated from this network."}
	nat_via:          {category: "network", label: "NAT via", meaning: "Outbound NAT is configured through this gateway."}
	nat_ip:           {category: "network", label: "NAT IP", meaning: "NAT is configured to use this address. Says nothing about which workloads egress through it."}
	private_zone_for: {category: "network", label: "private zone", meaning: "Private DNS zone is visible from this network. Name resolution only; not connectivity."}
	// traffic
	observed_flow: {category: "traffic", label: "flow seen", meaning: "Flow logs recorded traffic in the window. Absence of a flow is not proof of no traffic."}
	// dependency
	routes_to:     {category: "dependency", label: "routes to", meaning: "Load balancer target registration."}
	health_checks: {category: "dependency", label: "health check", meaning: "Load balancer health-checks this target."}
	reads_from:    {category: "dependency", label: "reads", meaning: "Configured to read from this resource."}
	writes_to:     {category: "dependency", label: "writes", meaning: "Configured to write to this resource."}
	publishes_to:  {category: "dependency", label: "publishes", meaning: "Configured to publish messages to this topic or queue."}
	consumes_from: {category: "dependency", label: "consumes", meaning: "Configured to consume from this queue or subscription."}
	subscribed_to: {category: "dependency", label: "subscribed", meaning: "Subscription attached to this topic."}
	replica_of:    {category: "dependency", label: "replica of", meaning: "Configured as a replica of this database."}
	scrapes:       {category: "dependency", label: "scrapes", meaning: "Monitoring configuration targets this resource."}
	// security
	allows_ingress_from: {category: "security", label: "allows from", meaning: "A rule permits ingress from this source. Permission is not observed traffic."}
	allows_egress_to:    {category: "security", label: "allows to", meaning: "A rule permits egress to this destination. Permission is not observed traffic."}
	applies_to:          {category: "security", label: "applies to", meaning: "Firewall rule targets this resource (by tag or service account)."}
	encrypted_by:        {category: "security", label: "encrypted by", meaning: "Data at rest is encrypted with this key."}
	exposed_to:          {category: "security", label: "exposed to", meaning: "Configured to accept connections from this external range."}
	// iam
	runs_as:     {category: "iam", label: "runs as", meaning: "Workload identity is this service account."}
	has_policy:  {category: "iam", label: "policy", meaning: "Policy is attached. Effective permission not evaluated."}
	grants_on:   {category: "iam", label: "grants on", meaning: "Policy statement names this resource. Conditions, boundaries and SCPs not evaluated."}
	iam_binding: {category: "iam", label: "binding", meaning: "IAM binding grants a role on this resource. Inherited, conditional and deny policies not evaluated."}
}

for id, e in _graphEdgeTypes {
	"graph_edge_type_\(id)": {head: {rel: "graph_edge_type", args: {type: id, category: e.category, label: e.label, meaning: e.meaning}}}
}

// pudl's own provider: address ranges and principals a rule names as an edge
// end (0.0.0.0/0, "*", allUsers). They are not cataloged resources, so their
// display info is declared here rather than in a schema.
graph_provider_external: {head: {rel: "graph_provider", args: {provider: "external", label: "External"}}}

graph_resource_type_external_cidr:      {head: {rel: "graph_resource_type", args: {type: "external.cidr", code: "CIDR", label: "External address range", class: "external"}}}
graph_resource_type_external_principal: {head: {rel: "graph_resource_type", args: {type: "external.principal", code: "EXT", label: "External principal", class: "external"}}}

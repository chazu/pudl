package rules

// GCP resource graph edges, read by `pudl graph export` and the web explorer.
// Each rule derives the reserved relation
//   graph_edge(type, source, target, basis)
// from the pack's projected facts. source is the record's own resource_id;
// target is the reference the record holds (a trimmed selfLink path, a KMS
// key name, an address URL), which the exporter resolves through each
// type's identity and `_pudl.graph.refs`. A target that was never collected
// stays a visible "not in inventory" reference. Types are the web explorer's
// relationship vocabulary (web/src/model/relationshipTypes.ts). basis is
// "observed": every edge is read from gcloud output.

gcp_graph_subnet_network: {
	head: {rel: "graph_edge", args: {type: "in_network", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_subnet", args: {resource_id: "$R", network: "$N"}}]
}

gcp_graph_firewall_network: {
	head: {rel: "graph_edge", args: {type: "in_network", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_firewall_network", args: {resource_id: "$R", network: "$N"}}]
}

gcp_graph_route_network: {
	head: {rel: "graph_edge", args: {type: "in_network", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_route_network", args: {resource_id: "$R", network: "$N"}}]
}

gcp_graph_router_network: {
	head: {rel: "graph_edge", args: {type: "in_network", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_router", args: {resource_id: "$R", network: "$N"}}]
}

gcp_graph_router_nat_ip: {
	head: {rel: "graph_edge", args: {type: "nat_ip", source: "$R", target: "$A", basis: "observed"}}
	body: [{rel: "gcp_router_nat_ip", args: {resource_id: "$R", address: "$A"}}]
}

gcp_graph_gke_subnet: {
	head: {rel: "graph_edge", args: {type: "in_subnet", source: "$R", target: "$S", basis: "observed"}}
	body: [{rel: "gcp_gke_cluster", args: {resource_id: "$R", subnetwork: "$S"}}]
}

gcp_graph_gke_key: {
	head: {rel: "graph_edge", args: {type: "encrypted_by", source: "$R", target: "$K", basis: "observed"}}
	body: [{rel: "gcp_gke_secrets_key", args: {resource_id: "$R", key: "$K"}}]
}

gcp_graph_sql_network: {
	head: {rel: "graph_edge", args: {type: "private_ip_in", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_sql_network", args: {resource_id: "$R", network: "$N"}}]
}

gcp_graph_dns_zone_network: {
	head: {rel: "graph_edge", args: {type: "private_zone_for", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_dns_zone_network", args: {resource_id: "$R", network: "$N"}}]
}

gcp_graph_subscription_topic: {
	head: {rel: "graph_edge", args: {type: "subscribed_to", source: "$R", target: "$T", basis: "observed"}}
	body: [{rel: "gcp_subscription_topic", args: {resource_id: "$R", topic: "$T"}}]
}

// ---- Firewall rules: ingress sources ----
// An enabled INGRESS rule with at least one allowed protocol permits traffic
// from each source range. A permission is not observed traffic. Ranges such
// as 0.0.0.0/0 become external "address range" nodes.

gcp_graph_firewall_source: {
	head: {rel: "graph_edge", args: {type: "allows_ingress_from", source: "$R", target: "$S", basis: "observed"}}
	body: [
		{rel: "gcp_firewall_source", args: {resource_id: "$R", range: "$S", direction: "INGRESS", disabled: false}},
		{rel: "gcp_firewall_allow", args: {resource_id: "$R"}},
	]
}

gcp_graph_firewall_port: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$S", key: "port", value: "$P"}}
	body: [
		{rel: "gcp_firewall_source", args: {resource_id: "$R", range: "$S", direction: "INGRESS", disabled: false}},
		{rel: "gcp_firewall_allow", args: {resource_id: "$R", port: "$P"}},
	]
}

gcp_graph_firewall_protocol: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$S", key: "protocol", value: "$P"}}
	body: [
		{rel: "gcp_firewall_source", args: {resource_id: "$R", range: "$S", direction: "INGRESS", disabled: false}},
		{rel: "gcp_firewall_allow", args: {resource_id: "$R", protocol: "$P"}},
	]
}

// ---- Compute instances ----

gcp_graph_instance_subnet: {
	head: {rel: "graph_edge", args: {type: "in_subnet", source: "$R", target: "$S", basis: "observed"}}
	body: [{rel: "gcp_instance_nic", args: {resource_id: "$R", subnetwork: "$S"}}]
}

// The service account an instance runs as. Service accounts are not
// collected by this pack, so they show as references not in the inventory.
gcp_graph_instance_sa: {
	head: {rel: "graph_edge", args: {type: "runs_as", source: "$R", target: "$A", basis: "observed"}}
	body: [{rel: "gcp_instance_sa", args: {resource_id: "$R", email: "$A"}}]
}

// Which firewall rules apply to which instances, as GCP decides it: an
// enabled rule with no target tags and no target service accounts applies
// to every instance with an interface in its network; a rule with target
// tags applies to instances carrying one of them, in its network. (Rules
// targeting service accounts are not matched here.) Applying is not
// allowing: the rule may deny, and priorities are not evaluated.
gcp_graph_firewall_applies_all: {
	head: {rel: "graph_edge", args: {type: "applies_to", source: "$F", target: "$I", basis: "observed"}}
	body: [
		{rel: "gcp_firewall_network", args: {resource_id: "$F", network: "$N", targeted: false, sa_targeted: false, disabled: false}},
		{rel: "gcp_instance_nic", args: {resource_id: "$I", network: "$N"}},
	]
}

gcp_graph_firewall_applies_tag: {
	head: {rel: "graph_edge", args: {type: "applies_to", source: "$F", target: "$I", basis: "observed"}}
	body: [
		{rel: "gcp_firewall_network", args: {resource_id: "$F", network: "$N", disabled: false}},
		{rel: "gcp_firewall_target_tag", args: {resource_id: "$F", tag: "$T"}},
		{rel: "gcp_instance_tag", args: {resource_id: "$I", tag: "$T"}},
		{rel: "gcp_instance_nic", args: {resource_id: "$I", network: "$N"}},
	]
}

gcp_graph_firewall_applies_tag_property: {
	head: {rel: "graph_edge_property", args: {type: "applies_to", source: "$F", target: "$I", key: "matched_tag", value: "$T"}}
	body: [
		{rel: "gcp_firewall_network", args: {resource_id: "$F", network: "$N", disabled: false}},
		{rel: "gcp_firewall_target_tag", args: {resource_id: "$F", tag: "$T"}},
		{rel: "gcp_instance_tag", args: {resource_id: "$I", tag: "$T"}},
		{rel: "gcp_instance_nic", args: {resource_id: "$I", network: "$N"}},
	]
}

// ---- IAM ----
// A project IAM policy binding grants a role to a member. The edge runs from
// the member (a collected service account, or a reference such as
// user:alice@example.com that pudl did not collect) to the project's policy
// node, with the role as a property. Inherited policies, conditions and deny
// policies are not evaluated: this is what the policy says, not effective
// access.

gcp_graph_iam_binding: {
	head: {rel: "graph_edge", args: {type: "iam_binding", source: "$M", target: "$R", basis: "observed"}}
	body: [{rel: "gcp_iam_binding", args: {resource_id: "$R", member: "$M"}}]
}

gcp_graph_iam_binding_role: {
	head: {rel: "graph_edge_property", args: {type: "iam_binding", source: "$M", target: "$R", key: "role", value: "$X"}}
	body: [{rel: "gcp_iam_binding", args: {resource_id: "$R", member: "$M", role: "$X"}}]
}

gcp_graph_iam_binding_condition: {
	head: {rel: "graph_edge_property", args: {type: "iam_binding", source: "$M", target: "$R", key: "conditional", value: "yes"}}
	body: [{rel: "gcp_iam_binding", args: {resource_id: "$R", member: "$M", conditional: true}}]
}

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

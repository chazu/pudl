package rules

// GCP resource graph edges, read by `pudl graph export` and the web explorer.
// Each rule derives the reserved relation
//   graph_edge(type, source, target, basis)
// from the pack's projected facts. source is the record's own resource_id;
// target is the reference the record holds (a trimmed selfLink path, a KMS
// key name, an address URL), which the exporter resolves through each
// type's identity and `_pudl.graph.refs`. A target that was never collected
// stays a visible "not in inventory" reference. Types are the web explorer's
// relationship vocabulary (pudl/rules/graph_vocabulary.cue). basis is
// "observed": every edge is read from collected records.

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

// A Kubernetes object lives in its GKE cluster, matched by project,
// location and name: the cluster's selfLink spells a zonal location as
// zones/, the object's asset name as locations/.
gcp_graph_k8s_deployment_cluster: {
	head: {rel: "graph_edge", args: {type: "in_cluster", source: "$R", target: "$C", basis: "observed"}}
	body: [
		{rel: "gcp_k8s_deployment", args: {resource_id: "$R", project: "$P", location: "$L", cluster: "$N"}},
		{rel: "gcp_gke_cluster", args: {resource_id: "$C", project: "$P", location: "$L", name: "$N"}},
	]
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

// A subscription forwards messages it cannot deliver to its dead-letter topic.
gcp_graph_subscription_dead_letter: {
	head: {rel: "graph_edge", args: {type: "publishes_to", source: "$R", target: "$T", basis: "observed"}}
	body: [{rel: "gcp_subscription_dead_letter", args: {resource_id: "$R", topic: "$T"}}]
}

gcp_graph_topic_key: {
	head: {rel: "graph_edge", args: {type: "encrypted_by", source: "$R", target: "$K", basis: "observed"}}
	body: [{rel: "gcp_topic_key", args: {resource_id: "$R", key: "$K"}}]
}

// The account a Cloud Run service runs as. Without one it runs as the
// Compute Engine default account, which no record names, so there is no edge.
gcp_graph_run_service_sa: {
	head: {rel: "graph_edge", args: {type: "runs_as", source: "$R", target: "$A", basis: "observed"}}
	body: [{rel: "gcp_run_service_sa", args: {resource_id: "$R", email: "$A"}}]
}

// Network peerings, with the peering's state as a property. Each side of a
// peering is its own record, so an active peering shows as two edges.
gcp_graph_network_peering: {
	head: {rel: "graph_edge", args: {type: "peers_with", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_network_peering", args: {resource_id: "$R", network: "$N"}}]
}

gcp_graph_network_peering_state: {
	head: {rel: "graph_edge_property", args: {type: "peers_with", source: "$R", target: "$N", key: "state", value: "$S"}}
	body: [{rel: "gcp_network_peering", args: {resource_id: "$R", network: "$N", state: "$S"}}]
}

// What an address is attached to, as GCP reports it (users): an instance, a
// forwarding rule or the Cloud NAT router that also shows as nat_ip.
gcp_graph_address_user: {
	head: {rel: "graph_edge", args: {type: "attached_to", source: "$R", target: "$U", basis: "observed"}}
	body: [{rel: "gcp_address_user", args: {resource_id: "$R", user: "$U"}}]
}

// ---- Who may reach a public endpoint ----
// A Cloud SQL instance with a public IP accepts connections from its
// authorized networks; a GKE control plane with master authorized networks
// enforced accepts them from the listed ranges. Ranges become external
// address-range nodes. (A public GKE endpoint without enforced authorized
// networks is reachable from anywhere; that is a check, not an edge.)

gcp_graph_sql_authorized_network: {
	head: {rel: "graph_edge", args: {type: "allows_ingress_from", source: "$R", target: "$C", basis: "observed"}}
	body: [{rel: "gcp_sql_authorized_network", args: {resource_id: "$R", range: "$C", public: true}}]
}

gcp_graph_gke_authorized_network: {
	head: {rel: "graph_edge", args: {type: "allows_ingress_from", source: "$R", target: "$C", basis: "observed"}}
	body: [{rel: "gcp_gke_authorized_network", args: {resource_id: "$R", range: "$C", enabled: true}}]
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
// tags applies to instances carrying one of them, and a rule with target
// service accounts to instances running as one of them, in its network.
// Applying is not allowing: the rule may deny, and priorities are not
// evaluated.
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

gcp_graph_firewall_applies_sa: {
	head: {rel: "graph_edge", args: {type: "applies_to", source: "$F", target: "$I", basis: "observed"}}
	body: [
		{rel: "gcp_firewall_network", args: {resource_id: "$F", network: "$N", disabled: false}},
		{rel: "gcp_firewall_target_sa", args: {resource_id: "$F", email: "$A"}},
		{rel: "gcp_instance_sa", args: {resource_id: "$I", email: "$A"}},
		{rel: "gcp_instance_nic", args: {resource_id: "$I", network: "$N"}},
	]
}

gcp_graph_firewall_applies_sa_property: {
	head: {rel: "graph_edge_property", args: {type: "applies_to", source: "$F", target: "$I", key: "matched_service_account", value: "$A"}}
	body: [
		{rel: "gcp_firewall_network", args: {resource_id: "$F", network: "$N", disabled: false}},
		{rel: "gcp_firewall_target_sa", args: {resource_id: "$F", email: "$A"}},
		{rel: "gcp_instance_sa", args: {resource_id: "$I", email: "$A"}},
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
// An IAM policy binding grants a role to a member on one resource (a
// project, bucket, service account, topic and so on). The edge runs from the
// member (a collected service account, or a reference such as
// user:alice@example.com that pudl did not collect) to that resource's
// policy node, with the role as a property. Inherited policies, conditions
// and deny policies are not evaluated: this is what the policy says, not
// effective access.

gcp_graph_iam_binding: {
	head: {rel: "graph_edge", args: {type: "iam_binding", source: "$M", target: "$R", basis: "observed"}}
	body: [{rel: "gcp_iam_member", args: {resource_id: "$R", member: "$M"}}]
}

gcp_graph_iam_binding_role: {
	head: {rel: "graph_edge_property", args: {type: "iam_binding", source: "$M", target: "$R", key: "role", value: "$X"}}
	body: [{rel: "gcp_iam_member", args: {resource_id: "$R", member: "$M", role: "$X"}}]
}

gcp_graph_iam_binding_condition: {
	head: {rel: "graph_edge_property", args: {type: "iam_binding", source: "$M", target: "$R", key: "conditional", value: "yes"}}
	body: [{rel: "gcp_iam_member", args: {resource_id: "$R", member: "$M", conditional: true}}]
}

// ---- Keys, accounts, routes, push delivery, images and policies ----
// Each target is derived from the source record by a fact rewrite
// (`_pudl.facts` ... rewrite) in the type's schema.
gcp_graph_kms_key_ring: {
	head: {rel: "graph_edge", args: {type: "in_key_ring", source: "$R", target: "$K", basis: "observed"}}
	body: [{rel: "gcp_kms_key_ring", args: {resource_id: "$R", ring: "$K"}}]
}
gcp_graph_bucket_key: {
	head: {rel: "graph_edge", args: {type: "encrypted_by", source: "$R", target: "$K", basis: "observed"}}
	body: [{rel: "gcp_bucket_key", args: {resource_id: "$R", key: "$K"}}]
}
gcp_graph_sql_key: {
	head: {rel: "graph_edge", args: {type: "encrypted_by", source: "$R", target: "$K", basis: "observed"}}
	body: [{rel: "gcp_sql_key", args: {resource_id: "$R", key: "$K"}}]
}
// The accounts a GKE cluster's nodes run as, with the node pool as a
// property. Pools on the Compute Engine default account have no edge.
gcp_graph_gke_node_pool_sa: {
	head: {rel: "graph_edge", args: {type: "runs_as", source: "$R", target: "$A", basis: "observed"}}
	body: [{rel: "gcp_gke_node_pool_sa", args: {resource_id: "$R", email: "$A"}}]
}
gcp_graph_gke_node_pool_sa_pool: {
	head: {rel: "graph_edge_property", args: {type: "runs_as", source: "$R", target: "$A", key: "node_pool", value: "$P"}}
	body: [{rel: "gcp_gke_node_pool_sa", args: {resource_id: "$R", email: "$A", pool: "$P"}}]
}
// Routes to an instance, a VPN tunnel or an internal load balancer. The pack
// collects instances; tunnels and forwarding rules stay references.
gcp_graph_route_next_hop: {
	head: {rel: "graph_edge", args: {type: "routes_via", source: "$R", target: "$H", basis: "observed"}}
	body: [{rel: "gcp_route_next_hop", args: {resource_id: "$R", hop: "$H"}}]
}
gcp_graph_route_next_hop_vpn: {
	head: {rel: "graph_edge", args: {type: "routes_via", source: "$R", target: "$H", basis: "observed"}}
	body: [{rel: "gcp_route_next_hop_vpn", args: {resource_id: "$R", hop: "$H"}}]
}
gcp_graph_route_next_hop_ilb: {
	head: {rel: "graph_edge", args: {type: "routes_via", source: "$R", target: "$H", basis: "observed"}}
	body: [{rel: "gcp_route_next_hop_ilb", args: {resource_id: "$R", hop: "$H"}}]
}
// A push subscription delivers to the endpoint origin it names; a Cloud Run
// service's URL resolves it. A custom domain mapped to a service does not.
gcp_graph_subscription_push: {
	head: {rel: "graph_edge", args: {type: "pushes_to", source: "$R", target: "$E", basis: "observed"}}
	body: [{rel: "gcp_subscription_push", args: {resource_id: "$R", endpoint: "$E"}}]
}
gcp_graph_run_service_image: {
	head: {rel: "graph_edge", args: {type: "runs_image", source: "$R", target: "$I", basis: "observed"}}
	body: [{rel: "gcp_run_service_image", args: {resource_id: "$R", image: "$I"}}]
}
gcp_graph_service_account_key: {
	head: {rel: "graph_edge", args: {type: "key_of", source: "$R", target: "$A", basis: "observed"}}
	body: [{rel: "gcp_service_account_key_account", args: {resource_id: "$R", email: "$A"}}]
}
// An IAM policy grants on the resource it is attached to.
gcp_graph_iam_policy_resource: {
	head: {rel: "graph_edge", args: {type: "grants_on", source: "$R", target: "$T", basis: "observed"}}
	body: [{rel: "gcp_iam_policy_resource", args: {resource_id: "$R", target: "$T"}}]
}

// Graph vocabulary for this pack (facts: rules with no body). Every edge
// type above is a pudl built-in (pudl/rules/graph_vocabulary.cue). A
// record's project names a gcp.project, which the pack does not collect, so
// its display info is declared here.
graph_provider_gcp:              {head: {rel: "graph_provider", args: {provider: "gcp", label: "Google Cloud"}}}
graph_account_type_gcp:          {head: {rel: "graph_account_type", args: {provider: "gcp", type: "gcp.project"}}}
graph_resource_type_gcp_project: {head: {rel: "graph_resource_type", args: {type: "gcp.project", code: "PROJ", label: "Project", class: "admin"}}}

// Edge types no built-in fits.
graph_edge_type_in_key_ring: {head: {rel: "graph_edge_type", args: {type: "in_key_ring", category: "containment",
	label: "in key ring", meaning: "A KMS key defined in this key ring."}}}
graph_edge_type_key_of: {head: {rel: "graph_edge_type", args: {type: "key_of", category: "iam",
	label: "key of", meaning: "A user-managed key that authenticates as this service account."}}}
graph_edge_type_pushes_to: {head: {rel: "graph_edge_type", args: {type: "pushes_to", category: "dependency",
	label: "pushes to", meaning: "A push subscription delivers messages to this endpoint. Delivery success is not observed."}}}
graph_edge_type_runs_image: {head: {rel: "graph_edge_type", args: {type: "runs_image", category: "dependency",
	label: "runs image", meaning: "Configured to run this container image (any tag or digest of it)."}}}

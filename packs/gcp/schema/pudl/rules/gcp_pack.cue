package rules

// GCP content pack rules. Single-resource best practices are policies
// (schema/gcp/policies.cue); these rules only threshold policy results or span
// two resource types. Relations come from the pack's fact projection.

// gcp_policy_noncompliant_high — a resource whose result for a policy has a
// high or critical violation. Checks select the population with evidence, so
// the rule needs no type filter. The head carries the severity so a policy
// waiver can tell which violations a finding stands for.
gcp_policy_noncompliant_high: {
	head: {rel: "gcp_policy_noncompliant_high", args: {resource: "$R", policy: "$P", severity: "high"}}
	body: [{rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "high"}}]
}

gcp_policy_noncompliant_critical: {
	head: {rel: "gcp_policy_noncompliant_high", args: {resource: "$R", policy: "$P", severity: "critical"}}
	body: [{rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "critical"}}]
}

// gcp_nat_ip_not_in_use — a Cloud NAT gateway configured with an external
// address that GCP reports as RESERVED, so the NAT is not using it.
gcp_nat_ip_not_in_use: {
	head: {rel: "gcp_nat_ip_not_in_use", args: {router: "$R", nat: "$N", address: "$A"}}
	body: [
		{rel: "gcp_router", args: {entry_id: "$E", self_link: "$R"}},
		{rel: "gcp_router_nat_ip", args: {entry_id: "$E", nat: "$N", address: "$A"}},
		{rel: "gcp_address", args: {self_link: "$A", status: "RESERVED"}},
	]
}

// gcp_gke_subnet_without_private_access — a cluster with private nodes in a
// subnet without Private Google Access: its nodes cannot reach Google APIs
// (image pulls from Artifact Registry, logging) without a NAT.
gcp_gke_subnet_without_private_access: {
	head: {rel: "gcp_gke_subnet_without_private_access", args: {cluster: "$C", subnet: "$S"}}
	body: [
		{rel: "gcp_gke_cluster", args: {self_link: "$C", subnetwork: "$S", private_nodes: true}},
		{rel: "gcp_subnet", args: {path: "$S", private_google_access: false}},
	]
}

// gcp_gke_secrets_key_unusable (one rule per state) — application-layer
// secrets encryption uses a KMS key whose primary version is disabled or
// destroyed, so the cluster cannot decrypt its etcd secrets.
gcp_gke_secrets_key_disabled: {
	head: {rel: "gcp_gke_secrets_key_unusable", args: {cluster: "$C", key: "$K"}}
	body: [
		{rel: "gcp_gke_cluster", args: {entry_id: "$E", self_link: "$C"}},
		{rel: "gcp_gke_secrets_key", args: {entry_id: "$E", key: "$K", state: "ENCRYPTED"}},
		{rel: "gcp_kms_key", args: {name: "$K", primary_state: "DISABLED"}},
	]
}

gcp_gke_secrets_key_destroyed: {
	head: {rel: "gcp_gke_secrets_key_unusable", args: {cluster: "$C", key: "$K"}}
	body: [
		{rel: "gcp_gke_cluster", args: {entry_id: "$E", self_link: "$C"}},
		{rel: "gcp_gke_secrets_key", args: {entry_id: "$E", key: "$K", state: "ENCRYPTED"}},
		{rel: "gcp_kms_key", args: {name: "$K", primary_state: "DESTROYED"}},
	]
}

gcp_gke_secrets_key_destroy_scheduled: {
	head: {rel: "gcp_gke_secrets_key_unusable", args: {cluster: "$C", key: "$K"}}
	body: [
		{rel: "gcp_gke_cluster", args: {entry_id: "$E", self_link: "$C"}},
		{rel: "gcp_gke_secrets_key", args: {entry_id: "$E", key: "$K", state: "ENCRYPTED"}},
		{rel: "gcp_kms_key", args: {name: "$K", primary_state: "DESTROY_SCHEDULED"}},
	]
}

// ---- Impact: what depends on this network ----

// gcp_depends_on_network(network, resource, kind) — every cataloged resource
// that uses a cataloged VPC network: its subnets, firewall rules, routes and
// Cloud Routers, GKE clusters, Cloud SQL instances with a private IP on it,
// and private DNS zones visible to it. The network is in path form
// (projects/P/global/networks/N); resource is a selfLink, or the zone name
// for DNS. One rule per kind.
//   pudl query gcp_depends_on_network network=projects/my-project/global/networks/prod-vpc
gcp_depends_on_network_subnet: {
	head: {rel: "gcp_depends_on_network", args: {network: "$N", resource: "$R", kind: "subnet"}}
	body: [
		{rel: "gcp_network", args: {path: "$N"}},
		{rel: "gcp_subnet", args: {path: "$R", network: "$N"}},
	]
}

gcp_depends_on_network_firewall: {
	head: {rel: "gcp_depends_on_network", args: {network: "$N", resource: "$R", kind: "firewall"}}
	body: [
		{rel: "gcp_network", args: {path: "$N"}},
		{rel: "gcp_firewall_network", args: {self_link: "$R", network: "$N"}},
	]
}

gcp_depends_on_network_route: {
	head: {rel: "gcp_depends_on_network", args: {network: "$N", resource: "$R", kind: "route"}}
	body: [
		{rel: "gcp_network", args: {path: "$N"}},
		{rel: "gcp_route_network", args: {self_link: "$R", network: "$N"}},
	]
}

gcp_depends_on_network_router: {
	head: {rel: "gcp_depends_on_network", args: {network: "$N", resource: "$R", kind: "router"}}
	body: [
		{rel: "gcp_network", args: {path: "$N"}},
		{rel: "gcp_router", args: {self_link: "$R", network: "$N"}},
	]
}

gcp_depends_on_network_gke: {
	head: {rel: "gcp_depends_on_network", args: {network: "$N", resource: "$R", kind: "gke_cluster"}}
	body: [
		{rel: "gcp_network", args: {path: "$N"}},
		{rel: "gcp_gke_cluster", args: {self_link: "$R", network: "$N"}},
	]
}

gcp_depends_on_network_sql: {
	head: {rel: "gcp_depends_on_network", args: {network: "$N", resource: "$R", kind: "sql_instance"}}
	body: [
		{rel: "gcp_network", args: {path: "$N"}},
		{rel: "gcp_sql_network", args: {self_link: "$R", network: "$N"}},
	]
}

gcp_depends_on_network_dns: {
	head: {rel: "gcp_depends_on_network", args: {network: "$N", resource: "$R", kind: "dns_zone"}}
	body: [
		{rel: "gcp_network", args: {path: "$N"}},
		{rel: "gcp_dns_zone_network", args: {entry_id: "$E", network: "$N"}},
		{rel: "gcp_dns_zone", args: {entry_id: "$E", name: "$R"}},
	]
}

// ---- Service account keys (iam-hygiene) ----
// Enabled user-managed keys created more than 90 days ago: rotate them
// (CIS GCP 1.7). "now" is the evaluation time, or the --as-of time.
gcp_sa_key_older_than_90d: {
	head: {rel: "gcp_sa_key_older_than_90d", args: {key: "$N", account: "$A"}}
	body: [
		{rel: "gcp_service_account_key", args: {entry_id: "$E", name: "$N", valid_after: "<now-90d", disabled: false}},
		{rel: "gcp_service_account_key_account", args: {entry_id: "$E", email: "$A"}},
	]
}

// Enabled user-managed keys that never expire (validBeforeTime is GCP's
// far-future sentinel): set an expiry, e.g. with the organization policy
// constraints/iam.serviceAccountKeyExpiryHours.
gcp_sa_key_never_expires: {
	head: {rel: "gcp_sa_key_never_expires", args: {key: "$N", account: "$A"}}
	body: [
		{rel: "gcp_service_account_key", args: {entry_id: "$E", name: "$N", valid_before: "9999-12-31T23:59:59Z", disabled: false}},
		{rel: "gcp_service_account_key_account", args: {entry_id: "$E", email: "$A"}},
	]
}

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

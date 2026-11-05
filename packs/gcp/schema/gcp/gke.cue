package gcp

// GKE clusters as printed by `gcloud container clusters list --format=json`.
// Identity is the cluster selfLink. Node metadata can carry startup scripts and
// legacy basic-auth credentials can appear under masterAuth, so those values
// are redacted before anything is stored.
#GKECluster: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.container.cluster"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["resourceLabels.team"]}
		graph: _gcpGraph & {region: "location"}
		graph: display: {code: "GKE", label: "GKE cluster", class: "compute"}
		graph: network: via: ["in_subnet"]
		sensitive_fields: [
			"nodeConfig.metadata",
			"nodePools[*].config.metadata",
			"masterAuth.password",
			"masterAuth.clientKey",
		]
		// The cluster's API server, as a kube context names it (the
		// Kubernetes pack's cluster record, later). Only keys unique to
		// one cluster: the public IP and the DNS endpoint. Private
		// endpoints repeat across VPCs (live check,
		// 2026-11-05), and on a private cluster endpoint is the private
		// IP, so neither is a key.
		same_resource: [
			{kind: "k8s.cluster", key: host: "privateClusterConfig.publicEndpoint"},
			{kind: "k8s.cluster", key: host: "controlPlaneEndpointsConfig.dnsEndpointConfig.endpoint"},
		]
		facts: {
			gcp_gke_cluster: args: {
				project:   "project"
				name:      "name"
				location:  "location"
				self_link: "selfLink"
				// networkConfig.subnetwork is in path form; the top-level
				// subnetwork is a short name and does not join.
				subnetwork: "networkConfig.subnetwork"
				network:    "networkConfig.network"
				private_nodes: {path: "privateClusterConfig.enablePrivateNodes", default: false}
			}
			gcp_gke_secrets_key: args: {
				key: "databaseEncryption.keyName"
				state: {path: "databaseEncryption.state", default: "DECRYPTED"}
			}
			// One fact per range allowed to reach the control plane, when
			// master authorized networks are enforced.
			gcp_gke_authorized_network: args: {
				range: "masterAuthorizedNetworksConfig.cidrBlocks[*].cidrBlock"
				enabled: {path: "masterAuthorizedNetworksConfig.enabled", default: false}
			}
			// The account each node pool's nodes run as. "default" (the
			// Compute Engine default account, which no record names) is
			// not an email, so it yields no fact.
			gcp_gke_node_pool_sa: {
				each: "nodePools[*]"
				args: {
					pool: "name"
					email: {path: "config.serviceAccount", rewrite: [{match: "^([^@]+@[^@]+)$", replace: "$1"}]}
				}
			}
		}
	}
	project:   _gcpProjectID
	name:      string
	selfLink:  =~#"^https://container\.googleapis\.com/v1/projects/[^/]+/(locations|zones)/[^/]+/clusters/[^/]+$"# @pudl(match)
	location:  string
	endpoint?: string
	networkConfig?: {network?: string, subnetwork?: string, ...}
	privateClusterConfig?: {enablePrivateNodes?: bool, enablePrivateEndpoint?: bool, ...}
	masterAuthorizedNetworksConfig?: {enabled?: bool, ...}
	workloadIdentityConfig?: {workloadPool?: string, ...}
	databaseEncryption?: {state?: string, keyName?: string, ...}
	...
}

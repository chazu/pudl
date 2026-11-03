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
		facts: {
			gcp_gke_cluster: args: {
				project:   "project"
				name:      "name"
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

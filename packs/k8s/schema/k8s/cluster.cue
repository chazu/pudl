package k8s

// The cluster kubectl's current context reaches, printed by the pack's
// helper (populators/k8s/cluster.sh) from `kubectl config view --minify`
// and `kubectl version`. One record per run; to collect another cluster,
// switch context and run again.
//
// Identity is host, the API server's host: a context, a kubeconfig cluster
// name and a server URL are how one machine names a cluster, and the same
// cluster is named differently elsewhere. The host is also the k8s.cluster
// key, which links the record to the same cluster seen through a cloud
// provider: gcp.#GKECluster declares it from the public endpoint and the
// DNS endpoint. A context that reaches a cluster by private IP does not
// link (private endpoints repeat across VPCs).
#Cluster: {
	_pudl: {
		schema_type:   "base"
		resource_type: "k8s.cluster"
		identity_fields: ["host"]
		review: status: "draft"
		graph: display: {code: "K8S", label: "Kubernetes cluster", class: "compute"}
		same_resource: [{kind: "k8s.cluster", key: host: "host"}]
		facts: k8s_cluster: args: {
			host:           "host"
			context:        "context"
			cluster:        "cluster"
			server:         "server"
			server_version: "serverVersion"
		}
	}
	// The kubeconfig context and cluster entry names.
	context: string & !=""
	cluster: string & !=""
	// The API server URL, and its host: scheme, a :443 port and a trailing
	// slash removed.
	server: =~"^https?://" @pudl(match)
	host:   string & !=""
	// The server's gitVersion, e.g. v1.30.5-gke.1014001.
	serverVersion: string & !=""
}

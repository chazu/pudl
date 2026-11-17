package rules

// Kubernetes graph edges, read by `pudl graph export` and the web explorer.
// Targets are resource_ids of k8s records, so an edge needs both records.
// basis is "observed": every edge is read from collected records.

// Every object lives in the cluster it was collected from: its cluster
// field is the host, the identity of the k8s.#Cluster record. A GKE
// cluster with that host as its public or DNS endpoint is the same graph
// node (k8s.cluster), as is a deployment GCP also sees (k8s.object).
_k8sObjectRelations: [
	"k8s_deployment", "k8s_statefulset", "k8s_daemonset",
	"k8s_service", "k8s_ingress", "k8s_namespace", "k8s_node",
]
for relation in _k8sObjectRelations {
	"k8s_graph_\(relation)_cluster": {
		head: {rel: "graph_edge", args: {type: "in_cluster", source: "$R", target: "$C", basis: "observed"}}
		body: [
			{rel: relation, args: {resource_id: "$R", cluster: "$H"}},
			{rel: "k8s_cluster", args: {resource_id: "$C", host: "$H"}},
		]
	}
}

// An Ingress routes to each Service one of its paths names, in the
// Ingress's namespace. A path with a resource backend names no Service.
k8s_graph_ingress_service: {
	head: {rel: "graph_edge", args: {type: "routes_to", source: "$R", target: "$T", basis: "observed"}}
	body: [
		{rel: "k8s_ingress_backend", args: {cluster: "$C", namespace: "$N", name: "$I", service: "$S"}},
		{rel: "k8s_ingress", args: {resource_id: "$R", cluster: "$C", namespace: "$N", name: "$I"}},
		{rel: "k8s_service", args: {resource_id: "$T", cluster: "$C", namespace: "$N", name: "$S"}},
	]
}

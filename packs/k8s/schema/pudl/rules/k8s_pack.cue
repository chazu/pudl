package rules

// Kubernetes content pack rules. Relations come from the pack's fact
// projection (packs/k8s/schema/k8s).

// k8s_ingress_backend — the service and port an Ingress sends a host and
// path to, one tuple per path of a rule: k8s_ingress_path (per path, on the
// Ingress record) with the Ingress's cluster, namespace and name. host is
// "" for a rule without one (it matches every host); service and port are
// "" for a path with a resource backend.
k8s_ingress_backend: {
	head: {rel: "k8s_ingress_backend", args: {
		cluster: "$C", namespace: "$N", name: "$I", host: "$H", path: "$P", service: "$S", port: "$O"
	}}
	body: [
		{rel: "k8s_ingress", args: {resource_id: "$R", cluster: "$C", namespace: "$N", name: "$I"}},
		{rel: "k8s_ingress_path", args: {resource_id: "$R", host: "$H", path: "$P", service: "$S", port: "$O"}},
	]
}

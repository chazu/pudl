package k8s

// Namespaces and Nodes, the cluster-scoped objects, as `kubectl get -o json`
// prints them, shaped by the collectors (schema/models/k8s_objects.cue):
// metadata.managedFields dropped; for a Node, status keeps only addresses,
// nodeInfo, conditions, capacity and allocatable (images, the list of every
// image the node pulled, is dropped). The system namespaces themselves are
// not collected.
//
// Identity is the cluster and the name.

// The facts every cluster-scoped object projects, under its own relation.
_k8sClusterScopedArgs: {
	cluster: "cluster"
	name:    "metadata.name"
}

#Namespace: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.namespace"
		identity_fields: ["cluster", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "NS", label: "Kubernetes namespace", class: "admin"}
		sensitive_fields: [_k8sLastApplied]
		same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: k8s_namespace: args: _k8sClusterScopedArgs
	}
	apiVersion: "v1"        @pudl(match)
	kind:       "Namespace" @pudl(match)
	cluster:    string & !=""
	metadata:   {name: string, uid: string, ...}
	spec?:      {...}
	status?:    {...}
}

#Node: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.node"
		identity_fields: ["cluster", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "NODE", label: "Kubernetes node", class: "compute"}
		sensitive_fields: [_k8sLastApplied]
		// Condition types and reasons are CamelCase constants (node-problem-
		// detector names such as FrequentContainerdRestart), not secrets.
		// Messages are free text and stay scanned.
		secret_scan_ignore: ["status.conditions[*].type", "status.conditions[*].reason"]
		same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]
		// provider_id names the node's VM (gce://project/zone/name,
		// aws:///zone/i-...); "" when the cloud provider sets none.
		facts: k8s_node: args: _k8sClusterScopedArgs & {
			provider_id: {path: "spec.providerID", default: ""}
		}
	}
	apiVersion: "v1"   @pudl(match)
	kind:       "Node" @pudl(match)
	cluster:    string & !=""
	metadata:   {name: string, uid: string, ...}
	spec?:      {...}
	status?:    {...}
}

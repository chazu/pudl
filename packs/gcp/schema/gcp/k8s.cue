package gcp

// Kubernetes objects of GKE clusters, as Cloud Asset Inventory exports them
// (CAI only: gcloud cannot list them). The record is the object (metadata,
// spec, status) plus where it lives: project, location and cluster, read from
// the asset name, which is also the identity. CAI omits apiVersion and kind;
// the collector adds them. Objects in system namespaces (kube-*, gke-*, gmp-*)
// are not collected.
//
// The object's UID links it to the same object collected through kubectl (a
// Kubernetes pack): every Kubernetes object has one, on any distribution.

// Paths under a pod template that can hold credentials: env values, and the
// last-applied annotation, which repeats the whole spec.
_gcpK8sPodSensitive: [
	"spec.template.spec.containers[*].env[*].value",
	"spec.template.spec.initContainers[*].env[*].value",
	#"metadata.annotations."kubectl.kubernetes.io/last-applied-configuration""#,
]

#K8sDeployment: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.k8s.deployment"
		identity_fields: ["assetName"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["metadata.labels.team"]}
		graph: _gcpGraph & {name: "metadata.name", region: "location"}
		graph: display: {code: "DEP", label: "Kubernetes deployment", class: "compute"}
		sensitive_fields: _gcpK8sPodSensitive
		same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: gcp_k8s_deployment: args: {
			project:   "project"
			location:  "location"
			cluster:   "cluster"
			namespace: "metadata.namespace"
			name:      "metadata.name"
			replicas: {path: "spec.replicas", default: 1}
		}
	}
	project:   _gcpProjectID
	assetName: =~#"^//container\.googleapis\.com/projects/[^/]+/locations/[^/]+/clusters/[^/]+/k8s/namespaces/[^/]+/apps/deployments/[^/]+$"# @pudl(match)
	location:  string
	cluster:   string
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: string, namespace: string, uid: string, ...}
	spec?: {...}
	...
}

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

// A container of a pod template, its image reference split the way a
// registry resolves it: the host (docker.io when the reference names none),
// the tag and the digest, "" when the reference carries none.
_gcpK8sContainerArgs: {
	container: "name"
	image:     "image"
	registry: {path: "image", rewrite: [
		{match: "^([^/]*[.:][^/]*)/", replace: "$1"},
		{match: "^localhost/", replace: "localhost"},
		{match: ".*", replace: "docker.io"},
	]}
	tag: {path: "image", rewrite: [
		{match: "^(?:[^@]*/)?[^/:@]*:([^/@]+)(?:@.*)?$", replace: "$1"},
		{match: ".*", replace: ""},
	]}
	digest: {path: "image", rewrite: [
		{match: "@([^@]+)$", replace: "$1"},
		{match: ".*", replace: ""},
	]}
}

// The Artifact Registry image a container runs, named as gcp.#ContainerImage
// is (artifacts.cue); a container running an image from elsewhere yields
// no fact.
_gcpK8sImageArgs: {
	container: "name"
	image: {path: "image", rewrite: _gcpArtifactImageRewrite}
}

// The label that names the Argo CD Application managing an object: the one
// Argo adds (argocd.argoproj.io/instance), or the one it uses when its
// settings make it respect the Kubernetes recommended labels. A rewrite that
// accepts anything, so an object without the label yields no fact.
_gcpK8sInstanceLabel: {
	in:  string
	out: {path: "metadata.labels.\"\(in)\"", rewrite: [{match: "^(.+)$", replace: "$1"}]}
}

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
		facts: {
			gcp_k8s_deployment: args: {
				project:   "project"
				location:  "location"
				cluster:   "cluster"
				namespace: "metadata.namespace"
				name:      "metadata.name"
				replicas: {path: "spec.replicas", default: 1}
			}
			// One fact per container of the pod template. Init containers
			// have their own relations: a relation fans out over one list.
			gcp_k8s_deployment_container: {each: "spec.template.spec.containers[*]", args: _gcpK8sContainerArgs}
			gcp_k8s_deployment_init_container: {each: "spec.template.spec.initContainers[*]", args: _gcpK8sContainerArgs}
			gcp_k8s_deployment_image: {each: "spec.template.spec.containers[*]", args: _gcpK8sImageArgs}
			gcp_k8s_deployment_init_image: {each: "spec.template.spec.initContainers[*]", args: _gcpK8sImageArgs}
			// The Argo CD Application that manages the deployment, by name.
			// The rule gcp_k8s_deployment_argo_app (pudl/rules/gcp_pack.cue)
			// reads both labels.
			gcp_k8s_deployment_argo_instance: args: instance: (_gcpK8sInstanceLabel & {in: "argocd.argoproj.io/instance"}).out
			gcp_k8s_deployment_app_instance: args: instance:  (_gcpK8sInstanceLabel & {in: "app.kubernetes.io/instance"}).out
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

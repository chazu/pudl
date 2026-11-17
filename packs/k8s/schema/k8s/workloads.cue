package k8s

// Workloads: Deployments, StatefulSets and DaemonSets, as `kubectl get -o
// json` prints them, shaped by the collectors (schema/models/
// k8s_workloads.cue): metadata.managedFields dropped, every pod template
// annotation value replaced by "[REDACTED]". Objects in system namespaces
// (kube-system, kube-public, kube-node-lease, gke-*, gmp-*) are not
// collected.
//
// cluster is the host of the cluster the record was collected from, the
// identity of its k8s.#Cluster record. Identity is the cluster, namespace
// and name: they stay when an object is recreated; the UID does not. The
// UID links the record to the same object seen through a cloud provider
// (gcp.#K8sDeployment) or another pack.

// Paths of a workload that can hold credentials: container and init
// container env values (valueFrom holds references only and is kept),
// every element of their command and args (flags such as --password=...,
// connection strings), and the last-applied annotation, which repeats the
// whole object as applied.
_k8sWorkloadSensitive: [
	"spec.template.spec.containers[*].env[*].value",
	"spec.template.spec.initContainers[*].env[*].value",
	"spec.template.spec.containers[*].command[*]",
	"spec.template.spec.containers[*].args[*]",
	"spec.template.spec.initContainers[*].command[*]",
	"spec.template.spec.initContainers[*].args[*]",
	#"metadata.annotations."kubectl.kubernetes.io/last-applied-configuration""#,
]

// _#K8sWorkload is the part every workload record has.
_#K8sWorkload: {
	apiVersion: "apps/v1"
	cluster:    string & !=""
	metadata:   {name: string, namespace: string, uid: string, ...}
	spec?:      {...}
	status?:    {...}
}

// The facts every workload kind projects, under its own relation.
_k8sWorkloadArgs: {
	cluster:   "cluster"
	namespace: "metadata.namespace"
	name:      "metadata.name"
}

#Deployment: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.deployment"
		identity_fields: ["cluster", "metadata.namespace", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "DEP", label: "Kubernetes deployment", class: "compute"}
		sensitive_fields: _k8sWorkloadSensitive
		same_resource:    [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: k8s_deployment: args: _k8sWorkloadArgs & {
			replicas: {path: "spec.replicas", default: 1}
		}
	}
	_#K8sWorkload
	kind: "Deployment" @pudl(match)
}

#StatefulSet: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.statefulset"
		identity_fields: ["cluster", "metadata.namespace", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "STS", label: "Kubernetes stateful set", class: "compute"}
		sensitive_fields: _k8sWorkloadSensitive
		same_resource:    [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: k8s_statefulset: args: _k8sWorkloadArgs & {
			replicas: {path: "spec.replicas", default: 1}
		}
	}
	_#K8sWorkload
	kind: "StatefulSet" @pudl(match)
}

#DaemonSet: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.daemonset"
		identity_fields: ["cluster", "metadata.namespace", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "DS", label: "Kubernetes daemon set", class: "compute"}
		sensitive_fields: _k8sWorkloadSensitive
		same_resource:    [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: k8s_daemonset: args: _k8sWorkloadArgs
	}
	_#K8sWorkload
	kind: "DaemonSet" @pudl(match)
}

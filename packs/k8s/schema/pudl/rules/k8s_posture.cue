package rules

// Kubernetes posture: the rules the posture check family
// (schema/models/k8s_pack_checks.cue) queries. Each finding is one container
// or init container of a workload: resource_id (the workload record),
// cluster, namespace, kind, name and container; list ("containers" or
// "initContainers") tells the two apart. Facts come from the workload schemas (schema/k8s/workloads.cue).

// k8s_workload — every Deployment, StatefulSet and DaemonSet with its kind.
_k8sWorkloadRelations: {
	Deployment:  "k8s_deployment"
	StatefulSet: "k8s_statefulset"
	DaemonSet:   "k8s_daemonset"
}
for k, relation in _k8sWorkloadRelations {
	"k8s_workload_\(relation)": {
		head: {rel: "k8s_workload", args: {resource_id: "$R", cluster: "$C", namespace: "$N", kind: k, name: "$W"}}
		body: [{rel: relation, args: {resource_id: "$R", cluster: "$C", namespace: "$N", name: "$W"}}]
	}
}

// k8s_pod_container — every container and init container of a workload's
// pod template, with its workload, the pod spec list it is in, and the
// fields of k8s_container.
_k8sContainerFields: {
	image_pin: "$I", privileged: "$P"
	sets_run_as_non_root: "$SNR", run_as_non_root: "$NR"
	sets_run_as_user: "$SU", run_as_user: "$U"
	has_cpu_limit: "$CL", has_memory_limit: "$ML"
}
for relation, listName in {k8s_container: "containers", k8s_init_container: "initContainers"} {
	"k8s_pod_container_\(relation)": {
		head: {rel: "k8s_pod_container", args: _k8sContainerFields & {
			resource_id: "$R", cluster: "$C", namespace: "$N", kind: "$K", name: "$W", container: "$X", list: listName
		}}
		body: [
			{rel: "k8s_workload", args: {resource_id: "$R", cluster: "$C", namespace: "$N", kind: "$K", name: "$W"}},
			{rel: relation, args: _k8sContainerFields & {resource_id: "$R", container: "$X"}},
		]
	}
}

// The head every posture finding has.
_k8sPostureHead: {resource_id: "$R", cluster: "$C", namespace: "$N", kind: "$K", name: "$W", container: "$X", list: "$L"}

// k8s_privileged_container — a container with securityContext.privileged.
k8s_privileged_container: {
	head: {rel: "k8s_privileged_container", args: _k8sPostureHead}
	body: [{rel: "k8s_pod_container", args: _k8sPostureHead & {privileged: true}}]
}

// k8s_unpinned_image — a container whose image is tagged latest or carries
// no tag and no digest. An image pinned by digest passes, whatever its tag.
for pin in ["latest", "untagged"] {
	"k8s_unpinned_image_\(pin)": {
		head: {rel: "k8s_unpinned_image", args: _k8sPostureHead}
		body: [{rel: "k8s_pod_container", args: _k8sPostureHead & {image_pin: pin}}]
	}
}

// k8s_container_without_limits — a container without resources.limits.cpu,
// resources.limits.memory or both; missing says which.
_k8sMissingLimits: {
	cpu: {cpu: false, memory: true}
	memory: {cpu: true, memory: false}
	"cpu and memory": {cpu: false, memory: false}
}
for which, has in _k8sMissingLimits {
	"k8s_container_without_limits_\(has.cpu)_\(has.memory)": {
		head: {rel: "k8s_container_without_limits", args: _k8sPostureHead & {missing: which}}
		body: [{rel: "k8s_pod_container", args: _k8sPostureHead & {has_cpu_limit: has.cpu, has_memory_limit: has.memory}}]
	}
}

// k8s_container_may_run_as_root — a container that can run as UID 0. A
// container's securityContext overrides its pod's, field by field, as in
// Kubernetes. It fails when its effective runAsUser is 0, or when it has no
// effective runAsUser and its effective runAsNonRoot is not true. A
// non-zero runAsUser, or runAsNonRoot: true without a runAsUser, passes.
_k8sRootCases: {
	// The container runs as UID 0.
	container_uid_0: {container: {sets_run_as_user: true, run_as_user: 0}}
	// The pod runs as UID 0 and the container sets no runAsUser.
	pod_uid_0: {container: {sets_run_as_user: false}, pod: {sets_run_as_user: true, run_as_user: 0}}
	// No runAsUser; the container sets runAsNonRoot: false.
	container_root_allowed: {
		container: {sets_run_as_user: false, sets_run_as_non_root: true, run_as_non_root: false}
		pod: {sets_run_as_user: false}
	}
	// No runAsUser, and neither the container nor the pod sets runAsNonRoot: true.
	pod_root_allowed: {
		container: {sets_run_as_user: false, sets_run_as_non_root: false}
		pod: {sets_run_as_user: false, run_as_non_root: false}
	}
}
for id, c in _k8sRootCases {
	"k8s_container_may_run_as_root_\(id)": {
		head: {rel: "k8s_container_may_run_as_root", args: _k8sPostureHead}
		body: [
			{rel: "k8s_pod_container", args: _k8sPostureHead & c.container},
			if c.pod != _|_ {{rel: "k8s_pod_security", args: {resource_id: "$R"} & c.pod}},
		]
	}
}

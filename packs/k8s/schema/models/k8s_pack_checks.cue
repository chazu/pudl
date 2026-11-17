package models

import (
	sm "pudl.schemas/pudl/systemmodel@v0"
)

// Kubernetes content pack check families. Like the GCP pack's
// (gcp_pack_checks.cue), each family is checks-only: it reads the latest
// observation of each collector it names (evidence "model:<name>"), which
// must be at most an hour old. The rules are in
// schema/pudl/rules/k8s_posture.cue.
//
//   k8s-posture-checks          over k8s-deployments, k8s-statefulsets and
//                               k8s-daemonsets;
//   k8s-fixture-posture-checks  over their fixture collectors.
//
// A live collector observes one scope per cluster host, so "model:<name>"
// selects the scopes its latest run observed: the live family's verdict
// covers the cluster (or clusters) those runs read, and no other.

// The collectors the posture checks read: every workload kind.
_k8sPostureCollectors: ["deployments", "statefulsets", "daemonsets"]

// A check over the latest observations of the workload collectors.
_#K8sCheck: {
	_prefix: string
	sm.#Check
	evidence: [for c in _k8sPostureCollectors {"model:\(_prefix)\(c)"}]
	max_age: "1h"
	expect:  "empty"
}

// Container posture: one finding per container or init container of a
// workload, keyed by its cluster, namespace, kind, name and container.
_#K8sPosture: {
	_prefix: string
	let P = _prefix
	sm.#SystemModel
	name: "\(_prefix)posture-checks"
	depends_on: [for c in _k8sPostureCollectors {"\(_prefix)\(c)"}]
	checks: [
		_#K8sCheck & {
			_prefix:  P
			name:     "privileged-container"
			query:    "k8s_privileged_container"
			severity: "fail"
			message:  "Remove securityContext.privileged: true from the container; a privileged container has full access to its node"
		},
		_#K8sCheck & {
			_prefix:  P
			name:     "unpinned-image"
			query:    "k8s_unpinned_image"
			severity: "warn"
			message:  "Pin the container's image by digest or a version tag; :latest or no tag can run a different image on each pull"
		},
		_#K8sCheck & {
			_prefix:  P
			name:     "no-resource-limits"
			query:    "k8s_container_without_limits"
			severity: "warn"
			message:  "Set resources.limits.cpu and resources.limits.memory on the container (missing names the absent limits)"
		},
		_#K8sCheck & {
			_prefix:  P
			name:     "may-run-as-root"
			query:    "k8s_container_may_run_as_root"
			severity: "warn"
			message:  "Set runAsNonRoot: true or a non-zero runAsUser on the container or its pod"
		},
	]
}

#K8sPostureChecks: _#K8sPosture & {_prefix: "k8s-"}
#K8sFixturePostureChecks: _#K8sPosture & {_prefix: "k8s-fixture-"}

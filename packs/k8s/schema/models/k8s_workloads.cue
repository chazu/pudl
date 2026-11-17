package models

import (
	"list"
	"regexp"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// Workload collectors of the Kubernetes pack: Deployments, StatefulSets and
// DaemonSets (README: packs/k8s/README.md in the PUDL source).
//
//   k8s-deployments, k8s-statefulsets, k8s-daemonsets
//       record every object of the kind in the cluster of kubectl's current
//       context, outside the system namespaces;
//   k8s-fixture-deployments, k8s-fixture-statefulsets, k8s-fixture-daemonsets
//       read the bundled fixtures in .pudl/populators/k8s.
//
// Like k8s-cluster, each live collector observes one scope per cluster
// host (k8s/<host>/<kind>), complete.

// Kubernetes and cloud provider system namespaces hold built-in defaults,
// which the pack does not catalog (the GCP pack skips the same ones).
_k8sSystemNamespace: "^(kube-system|kube-public|kube-node-lease|gke-.*|gmp-.*)$"

// _k8sRedacted replaces a value the shape does not keep.
_k8sRedacted: "[REDACTED]"

// _k8sOmit copies the fields of _src except those named in _drop.
_k8sOmit: {
	_src:  _
	_drop: [...string]
	let d = _drop
	out: {for k, v in _src if !list.Contains(d, k) {(k): v}}
}

// _k8sPodTemplate is a pod template with the value of every annotation
// replaced by _k8sRedacted; the keys stay. Pod template annotations are
// read by agents and injectors, and can carry their configuration,
// credentials included. Env values are redacted by the schemas.
_k8sPodTemplate: {
	_src: _
	let t = _src
	out: {
		(_k8sOmit & {_src: t, _drop: ["metadata"]}).out
		if t.metadata != _|_ {
			metadata: {
				(_k8sOmit & {_src: t.metadata, _drop: ["annotations"]}).out
				if t.metadata.annotations != _|_ {
					annotations: {for k, _ in t.metadata.annotations {(k): _k8sRedacted}}
				}
			}
		}
	}
}

// _k8sWorkload is one workload record from a kubectl item: the item
// without metadata.managedFields, and its pod template through
// _k8sPodTemplate. cluster is set by the run.
_k8sWorkload: {
	_in: _
	let item = _in
	(_k8sOmit & {_src: item, _drop: ["metadata", "spec"]}).out
	metadata: (_k8sOmit & {_src: item.metadata, _drop: ["managedFields"]}).out
	if item.spec != _|_ {
		spec: {
			(_k8sOmit & {_src: item.spec, _drop: ["template"]}).out
			if item.spec.template != _|_ {
				template: (_k8sPodTemplate & {_src: item.spec.template}).out
			}
		}
	}
}

// _k8sWorkloadTransform unwraps the List `kubectl get -o json` prints,
// skips the objects in system namespaces and shapes the rest, as
// _k8sListTransform describes.
_k8sWorkloadTransform: {
	in:  _
	out: [for item in in.items if !regexp.Match(_k8sSystemNamespace, item.metadata.namespace) {_k8sWorkload & {_in: item}}]
}

// _k8sLive is a live collector of one kind: _k8sGet on the current
// context's cluster, each item through _transform, cluster set on every
// record.
_k8sLive: {
	_resource:   string
	_namespaced: *true | bool
	_schema:     string
	_transform:  _
	let resource = _resource
	let namespaced = _namespaced
	let xf = _transform
	populate: sm.#CommandObserve & {
		schema: _schema
		each:   _k8sClusterEach
		runs: [{
			argv: (_k8sGet & {_resource: resource, _namespaced: namespaced}).argv
			set: cluster: "{{item}}"
			transform: xf
		}]
	}
}

// _k8sWorkloadLive is a live collector of one workload kind.
_k8sWorkloadLive: {
	_resource: string
	_schema:   string
	_k8sLive & {_transform: _k8sWorkloadTransform}
}

// The fixture cluster's host, as populators/k8s/cluster.json gives it: the
// public endpoint of the GCP fixture GKE cluster prod-private.
_k8sFixtureHost: "198.51.100.30"

// _k8sFixture reads a saved kubectl List of one kind, and the hostile List
// that plants values the shape and redaction must drop.
_k8sFixture: {
	_file:      string
	_schema:    string
	_transform: _
	let xf = _transform
	populate: sm.#CommandObserve & {
		schema: _schema
		runs: [for f in [_file, "\(_file)-hostile"] {
			argv: ["cat", "../../populators/k8s/\(f).json"]
			set: cluster: _k8sFixtureHost
			transform: xf
		}]
	}
}

// _k8sWorkloadFixture is the fixture collector of one workload kind.
_k8sWorkloadFixture: {
	_file:   string
	_schema: string
	_k8sFixture & {_transform: _k8sWorkloadTransform}
}

#K8sDeployments: sm.#SystemModel & {
	name:        "k8s-deployments"
	observation: {scope: "k8s/{{item}}/deployments", complete: true}
	_k8sWorkloadLive & {_resource: "deployments.apps", _schema: "k8s.#Deployment"}
}

#K8sStatefulSets: sm.#SystemModel & {
	name:        "k8s-statefulsets"
	observation: {scope: "k8s/{{item}}/statefulsets", complete: true}
	_k8sWorkloadLive & {_resource: "statefulsets.apps", _schema: "k8s.#StatefulSet"}
}

#K8sDaemonSets: sm.#SystemModel & {
	name:        "k8s-daemonsets"
	observation: {scope: "k8s/{{item}}/daemonsets", complete: true}
	_k8sWorkloadLive & {_resource: "daemonsets.apps", _schema: "k8s.#DaemonSet"}
}

#K8sFixtureDeployments: sm.#SystemModel & {
	name:        "k8s-fixture-deployments"
	observation: {scope: "fixture/k8s/deployments", complete: true}
	_k8sWorkloadFixture & {_file: "deployments", _schema: "k8s.#Deployment"}
}

#K8sFixtureStatefulSets: sm.#SystemModel & {
	name:        "k8s-fixture-statefulsets"
	observation: {scope: "fixture/k8s/statefulsets", complete: true}
	_k8sWorkloadFixture & {_file: "statefulsets", _schema: "k8s.#StatefulSet"}
}

#K8sFixtureDaemonSets: sm.#SystemModel & {
	name:        "k8s-fixture-daemonsets"
	observation: {scope: "fixture/k8s/daemonsets", complete: true}
	_k8sWorkloadFixture & {_file: "daemonsets", _schema: "k8s.#DaemonSet"}
}

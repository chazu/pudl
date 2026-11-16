package models

import (
	"list"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// Argo CD content pack models (README: packs/argocd/README.md in the PUDL
// source). The pack needs the Kubernetes pack (k8s_pack.cue): the live
// collectors use its cluster helper and kubectl run shape.
//
//   argocd-applications           records every Application in the cluster
//                                 of kubectl's current context;
//   argocd-applicationsets        records every ApplicationSet there;
//   argocd-fixture-applications,
//   argocd-fixture-applicationsets read the bundled fixtures in
//                                 .pudl/populators/argocd.
//
// The live collectors read the current context only, run kubectl get and
// nothing else, and declare complete: false, as the Kubernetes pack's
// collectors do: nothing is marked gone after a context switch.

// _argocdKept keeps the objects in the namespaces _argocdNamespaces names
// (argocd_settings.cue); an empty list keeps every namespace.
_argocdKept: {
	_ns: string
	ok:  len(_argocdNamespaces) == 0 || list.Contains(_argocdNamespaces, _ns)
}

// _argocdListTransform unwraps the List `kubectl get -o json` prints and
// shapes each item with _shape (an allow-list transform, argocd_shape.cue).
_argocdListTransform: {
	_shape: _
	in:     _
	out: [for item in in.items if (_argocdKept & {_ns: item.metadata.namespace}).ok {_shape & {_in: item}}]
}

// _argocdLive is a live collector of one Argo CD resource type.
_argocdLive: {
	_resource: string
	_schema:   string
	_shape:    _
	let resource = _resource
	let shape = _shape
	populate: sm.#CommandObserve & {
		schema: _schema
		each:   _k8sClusterEach
		runs: [{
			argv: (_k8sGet & {_resource: resource}).argv
			set: cluster: "{{item}}"
			transform: _argocdListTransform & {_shape: shape}
		}]
	}
}

// The fixture cluster: the host of the GCP fixture GKE cluster
// batch-private's DNS endpoint. Argo CD runs there and deploys to the
// fixture cluster prod-private by its server URL.
_argocdFixtureCluster: "gke-00000000000000000000000000000000-000000000000.us-central1.gke.goog"

// _argocdFixture reads a saved kubectl List of one resource type, and the
// hostile List that plants values the transform and redaction must drop.
_argocdFixture: {
	_file:   string
	_schema: string
	_shape:  _
	let shape = _shape
	populate: sm.#CommandObserve & {
		schema: _schema
		runs: [for f in [_file, "\(_file)-hostile"] {
			argv: ["cat", "../../populators/argocd/\(f).json"]
			set: cluster: _argocdFixtureCluster
			transform: _argocdListTransform & {_shape: shape}
		}]
	}
}

#ArgoCDApplications: sm.#SystemModel & {
	name: "argocd-applications"
	observation: {scope: "argocd/applications", complete: false}
	_argocdLive & {_resource: "applications.argoproj.io", _schema: "argocd.#Application", _shape: _argocdApplication}
}

#ArgoCDApplicationSets: sm.#SystemModel & {
	name: "argocd-applicationsets"
	observation: {scope: "argocd/applicationsets", complete: false}
	_argocdLive & {_resource: "applicationsets.argoproj.io", _schema: "argocd.#ApplicationSet", _shape: _argocdApplicationSet}
}

#ArgoCDFixtureApplications: sm.#SystemModel & {
	name: "argocd-fixture-applications"
	observation: {scope: "fixture/argocd/applications", complete: true}
	_argocdFixture & {_file: "applications", _schema: "argocd.#Application", _shape: _argocdApplication}
}

#ArgoCDFixtureApplicationSets: sm.#SystemModel & {
	name: "argocd-fixture-applicationsets"
	observation: {scope: "fixture/argocd/applicationsets", complete: true}
	_argocdFixture & {_file: "applicationsets", _schema: "argocd.#ApplicationSet", _shape: _argocdApplicationSet}
}

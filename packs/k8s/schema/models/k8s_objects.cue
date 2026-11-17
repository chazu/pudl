package models

import (
	"list"
	"regexp"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// Object collectors of the Kubernetes pack: Services, Ingresses,
// Namespaces and Nodes (README: packs/k8s/README.md in the PUDL source).
//
//   k8s-services, k8s-ingresses, k8s-namespaces, k8s-nodes
//       record every object of the kind in the cluster of kubectl's current
//       context, outside the system namespaces;
//   k8s-fixture-services, k8s-fixture-ingresses, k8s-fixture-namespaces,
//   k8s-fixture-nodes
//       read the bundled fixtures in .pudl/populators/k8s.
//
// Like k8s-cluster, each live collector observes one scope per cluster
// host (k8s/<host>/<kind>), complete.

// ---- Shapes.

// _k8sCredentialKey matches an annotation key that names a credential,
// such as nginx's auth-url and auth-secret.
_k8sCredentialKey: "(?i)(auth|secret|token|password|passwd|credential|key)"

// _k8sURLPattern splits a URL with a scheme into the scheme, the userinfo
// before the last "@" of the authority, if any, and the rest. (?s) lets the
// rest hold a newline. As the Argo CD pack's _argocdRepoURLPattern.
_k8sURLPattern: "(?s)^([A-Za-z][A-Za-z0-9+.-]*)://(?:([^/?#]*)@)?(.*)$"

// _k8sURL is a value without the userinfo it carries if it is a URL with a
// scheme: https://user:TOKEN@host/path becomes https://host/path. Any other
// value is kept as it is.
_k8sURL: {
	_url: string
	let u = _url
	out: string
	if u !~ _k8sURLPattern {out: u}
	if u =~ _k8sURLPattern {
		let m = regexp.FindSubmatch(_k8sURLPattern, u)
		out: m[1] + "://" + m[3]
	}
}

// _k8sAnnotations keeps every annotation key. The value of one whose key
// matches _k8sCredentialKey becomes _k8sRedacted; any other value goes
// through _k8sURL. The last-applied annotation is redacted by the schemas.
_k8sAnnotations: {
	_src: _
	out: {
		for k, v in _src {
			if k =~ _k8sCredentialKey {(k): _k8sRedacted}
			if k !~ _k8sCredentialKey {(k): (_k8sURL & {_url: v}).out}
		}
	}
}

// _k8sMetadata is metadata without managedFields. With _filter, the
// annotations go through _k8sAnnotations; otherwise they are kept.
_k8sMetadata: {
	_src:    _
	_filter: bool
	let m = _src
	let f = _filter
	out: {
		(_k8sOmit & {_src: m, _drop: ["managedFields", "annotations"]}).out
		if m.annotations != _|_ {
			if f {annotations: (_k8sAnnotations & {_src: m.annotations}).out}
			if !f {annotations: m.annotations}
		}
	}
}

// _k8sService is one Service record from a kubectl item. cluster is set by
// the run.
_k8sService: {
	_in: _
	let item = _in
	(_k8sOmit & {_src: item, _drop: ["metadata"]}).out
	metadata: (_k8sMetadata & {_src: item.metadata, _filter: true}).out
}

// _k8sIngressPath is one path of an Ingress rule, with the rule's host
// (_host) and its service backend's port number or name set as host and
// port, for the k8s_ingress_path fact.
_k8sIngressPath: {
	_src:  _
	_host: string
	let p = _src
	let h = _host
	out: {
		p
		host: h
		if p.backend.service.port.number != _|_ {port: p.backend.service.port.number}
		if p.backend.service.port.number == _|_ && p.backend.service.port.name != _|_ {
			port: p.backend.service.port.name
		}
	}
}

// _k8sIngressRule is one Ingress rule, every path through _k8sIngressPath.
_k8sIngressRule: {
	_src: _
	let r = _src
	let h = [if r.host != _|_ {r.host}, ""][0]
	out: {
		(_k8sOmit & {_src: r, _drop: ["http"]}).out
		if r.http != _|_ {
			http: {
				(_k8sOmit & {_src: r.http, _drop: ["paths"]}).out
				paths: [for p in r.http.paths {(_k8sIngressPath & {_src: p, _host: h}).out}]
			}
		}
	}
}

// _k8sIngress is one Ingress record from a kubectl item.
_k8sIngress: {
	_in: _
	let item = _in
	(_k8sOmit & {_src: item, _drop: ["metadata", "spec"]}).out
	metadata: (_k8sMetadata & {_src: item.metadata, _filter: true}).out
	if item.spec != _|_ {
		spec: {
			(_k8sOmit & {_src: item.spec, _drop: ["rules"]}).out
			if item.spec.rules != _|_ {
				rules: [for r in item.spec.rules {(_k8sIngressRule & {_src: r}).out}]
			}
		}
	}
}

// _k8sNamespace is one Namespace record from a kubectl item.
_k8sNamespace: {
	_in: _
	let item = _in
	(_k8sOmit & {_src: item, _drop: ["metadata"]}).out
	metadata: (_k8sMetadata & {_src: item.metadata, _filter: false}).out
}

// The status fields of a Node kept. The rest (images, the list of every
// image the node pulled; volumes; daemon endpoints; config) is dropped.
_k8sNodeStatus: ["addresses", "nodeInfo", "conditions", "capacity", "allocatable"]

// _k8sNode is one Node record from a kubectl item.
_k8sNode: {
	_in: _
	let item = _in
	(_k8sOmit & {_src: item, _drop: ["metadata", "status"]}).out
	metadata: (_k8sMetadata & {_src: item.metadata, _filter: false}).out
	if item.status != _|_ {
		status: {for k, v in item.status if list.Contains(_k8sNodeStatus, k) {(k): v}}
	}
}

// ---- Transforms: unwrap the List `kubectl get -o json` prints, skip what
// is in (or is) a system namespace, and shape each item with _shape.

_k8sNamespacedTransform: {
	_shape: _
	in:     _
	out: [for item in in.items if !regexp.Match(_k8sSystemNamespace, item.metadata.namespace) {_shape & {_in: item}}]
}

_k8sNamespaceTransform: {
	in: _
	out: [for item in in.items if !regexp.Match(_k8sSystemNamespace, item.metadata.name) {_k8sNamespace & {_in: item}}]
}

_k8sNodeTransform: {
	in: _
	out: [for item in in.items {_k8sNode & {_in: item}}]
}

// ---- Collectors.

#K8sServices: sm.#SystemModel & {
	name:        "k8s-services"
	observation: {scope: "k8s/{{item}}/services", complete: true}
	_k8sLive & {
		_resource:  "services"
		_schema:    "k8s.#Service"
		_transform: _k8sNamespacedTransform & {_shape: _k8sService}
	}
}

#K8sIngresses: sm.#SystemModel & {
	name:        "k8s-ingresses"
	observation: {scope: "k8s/{{item}}/ingresses", complete: true}
	_k8sLive & {
		_resource:  "ingresses.networking.k8s.io"
		_schema:    "k8s.#Ingress"
		_transform: _k8sNamespacedTransform & {_shape: _k8sIngress}
	}
}

#K8sNamespaces: sm.#SystemModel & {
	name:        "k8s-namespaces"
	observation: {scope: "k8s/{{item}}/namespaces", complete: true}
	_k8sLive & {
		_resource:   "namespaces"
		_namespaced: false
		_schema:     "k8s.#Namespace"
		_transform:  _k8sNamespaceTransform
	}
}

#K8sNodes: sm.#SystemModel & {
	name:        "k8s-nodes"
	observation: {scope: "k8s/{{item}}/nodes", complete: true}
	_k8sLive & {
		_resource:   "nodes"
		_namespaced: false
		_schema:     "k8s.#Node"
		_transform:  _k8sNodeTransform
	}
}

#K8sFixtureServices: sm.#SystemModel & {
	name:        "k8s-fixture-services"
	observation: {scope: "fixture/k8s/services", complete: true}
	_k8sFixture & {
		_file:      "services"
		_schema:    "k8s.#Service"
		_transform: _k8sNamespacedTransform & {_shape: _k8sService}
	}
}

#K8sFixtureIngresses: sm.#SystemModel & {
	name:        "k8s-fixture-ingresses"
	observation: {scope: "fixture/k8s/ingresses", complete: true}
	_k8sFixture & {
		_file:      "ingresses"
		_schema:    "k8s.#Ingress"
		_transform: _k8sNamespacedTransform & {_shape: _k8sIngress}
	}
}

#K8sFixtureNamespaces: sm.#SystemModel & {
	name:        "k8s-fixture-namespaces"
	observation: {scope: "fixture/k8s/namespaces", complete: true}
	_k8sFixture & {_file: "namespaces", _schema: "k8s.#Namespace", _transform: _k8sNamespaceTransform}
}

#K8sFixtureNodes: sm.#SystemModel & {
	name:        "k8s-fixture-nodes"
	observation: {scope: "fixture/k8s/nodes", complete: true}
	_k8sFixture & {_file: "nodes", _schema: "k8s.#Node", _transform: _k8sNodeTransform}
}

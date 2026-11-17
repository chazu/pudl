package models

import (
	sm "pudl.schemas/pudl/systemmodel@v0"
)

// Kubernetes content pack models (README: packs/k8s/README.md in the PUDL
// source).
//
//   k8s-cluster          records the cluster of kubectl's current context;
//   k8s-fixture-cluster  reads the bundled fixture in .pudl/populators/k8s.
//
// The live collectors read the current context only. To collect another
// cluster, switch context and run again. Each enumerates the current
// cluster's host with `each` and observes the scope k8s/<host>/<kind>
// (observation scope "k8s/{{item}}/<kind>"), complete: a run marks gone
// only objects that its own cluster no longer has, and a context switch
// leaves the other clusters' records as they are.

// ---- Shared by the pack's other collectors, and by packs that collect
// through kubectl (the Argo CD pack). All are in package models, which every
// installed pack shares.

// _k8sClusterHelper prints the current context's cluster as one object
// (populators/k8s/cluster.sh): its context and cluster entries, kubectl
// version's output and host, the API server's host.
_k8sClusterHelper: ["sh", "../../populators/k8s/cluster.sh"]

// _k8sClusterEach enumerates the current cluster's host, one input, so a
// collector can set it on every object it collects (set: cluster: "{{item}}")
// to point at the cluster record, and scope its observation by it.
_k8sClusterEach: {argv: _k8sClusterHelper, select: "host"}

// _k8sListTransform stores the items of the List object `kubectl get -o
// json` prints, one record each. Compose it with a per-item shape:
//
//	transform: {in: _, out: [for item in in.items {_shape & {_in: item}}]}
_k8sListTransform: {in: _, out: in.items}

// _k8sGet is a run that lists one resource type across all namespaces and
// unwraps the List. kubectl prints apiVersion and kind on every item. For a
// cluster-scoped type (namespaces, nodes), set _namespaced: false to leave
// out -A.
//
//	runs: [_k8sGet & {_resource: "deployments.apps"}]
_k8sGet: {
	_resource:   string & !=""
	_namespaced: *true | bool
	argv: ["kubectl", "get", _resource, if _namespaced {"-A"}, "-o", "json"]
	transform: _k8sListTransform
}

// ---- The cluster record.

// _k8sClusterTransform keeps the fields of k8s.#Cluster from the helper's
// object. Everything else (the client version, the kubeconfig user name) is
// dropped.
_k8sClusterTransform: {
	in: _
	out: {
		context:       in.context.name
		cluster:       in.cluster.name
		server:        in.cluster.cluster.server
		host:          in.host
		serverVersion: in.version.serverVersion.gitVersion
	}
}

#K8sCluster: sm.#SystemModel & {
	name: "k8s-cluster"
	observation: {scope: "k8s/{{item}}/cluster", complete: true}
	populate: sm.#CommandObserve & {
		schema: "k8s.#Cluster"
		each:   _k8sClusterEach
		runs: [{argv: _k8sClusterHelper, set: host: "{{item}}", transform: _k8sClusterTransform}]
	}
}

#K8sFixtureCluster: sm.#SystemModel & {
	name: "k8s-fixture-cluster"
	observation: {scope: "fixture/k8s/cluster", complete: true}
	populate: sm.#CommandObserve & {
		schema: "k8s.#Cluster"
		runs: [{argv: ["cat", "../../populators/k8s/cluster.json"], transform: _k8sClusterTransform}]
	}
}

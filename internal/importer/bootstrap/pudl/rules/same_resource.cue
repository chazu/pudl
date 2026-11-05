package rules

// Shared identity across schemas: records of different schemas that describe
// one real resource are linked, never merged. See
// docs/design/2026-11-05-shared-identity-across-schemas.md.
//
//	same_resource_kind(kind, label, meaning)  an identity kind (facts below; packs add their own)
//	same_resource_key(kind, key, resource_id) projected from a schema's _pudl.same_resource
//	same_resource(a, b, kind)                 a and b represent one resource of kind
//
// same_resource also has a = b rows (datalog has no variable inequality);
// consumers drop them. Packs may derive same_resource with their own rules
// when a key needs a join, for example objects linked through their linked
// clusters.

// Kinds that mean the same thing for every provider.
same_resource_kind_k8s_cluster: {
	head: {rel: "same_resource_kind", args: {kind: "k8s.cluster", label: "Kubernetes cluster",
		meaning: "A Kubernetes cluster, keyed by its API server host."}}
}

same_resource_kind_k8s_object: {
	head: {rel: "same_resource_kind", args: {kind: "k8s.object", label: "Kubernetes object",
		meaning: "A Kubernetes object, keyed by its linked cluster, namespace, group, kind and name."}}
}

// Resources holding an equal key of one kind are the same resource.
same_resource_by_key: {
	head: {rel: "same_resource", args: {a: "$A", b: "$B", kind: "$K"}}
	body: [
		{rel: "same_resource_key", args: {kind: "$K", key: "$V", resource_id: "$A"}},
		{rel: "same_resource_key", args: {kind: "$K", key: "$V", resource_id: "$B"}},
	]
}

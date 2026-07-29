package k8s

// Resource is the stable inventory envelope emitted by mu's Kubernetes
// observer. The payload remains open because Kubernetes has an extensible API;
// users can layer a kind-specific schema on top when they need stricter
// validation without making the built-in observer unusable on a new kind.
#Resource: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.resource"
		identity_fields: ["kind", "metadata.namespace", "metadata.name"]
		tracked_fields:  ["apiVersion", "kind", "metadata", "spec", "status"]
	}

	apiVersion: string
	kind:       string
	metadata: {
		name:      string
		namespace?: string
		uid?:       string
		labels?:    [string]: string
		annotations?: [string]: string
		...
	}
	spec?:   {...}
	status?: {...}
	...
}

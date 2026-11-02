package gcp

// Serving and messaging: Cloud Run, Cloud DNS and Pub/Sub.

// `gcloud run services list --format=json` prints Knative Service objects.
// metadata.selfLink omits the region, so identity is the project number
// (metadata.namespace), the location label and the name. Environment values
// and the creator/modifier e-mail annotations are redacted before storage;
// secret references (valueFrom) are kept.
#RunService: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.run.service"
		identity_fields: [
			"metadata.namespace",
			"metadata.labels.\"cloud.googleapis.com/location\"",
			"metadata.name",
		]
		review: status: "draft"
		sensitive_fields: [
			"spec.template.spec.containers[*].env[*].value",
			"metadata.annotations.\"serving.knative.dev/creator\"",
			"metadata.annotations.\"serving.knative.dev/lastModifier\"",
		]
	}
	apiVersion: "serving.knative.dev/v1"
	kind:       "Service"
	project:    string
	metadata: {
		name:      string
		namespace: string
		labels: "cloud.googleapis.com/location": string
		...
	}
	...
}

// `gcloud dns managed-zones list --format=json`. Zone names are unique per
// project and gcloud prints no selfLink, so identity is project and name.
#DNSManagedZone: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.dns.managed_zone"
		identity_fields: ["project", "name"]
		review: status: "draft"
	}
	kind:        "dns#managedZone"
	project:     string
	name:        string
	dnsName:     string
	visibility?: "public" | "private"
	...
}

// `gcloud pubsub topics list --format=json`. Identity is the fully qualified
// name projects/P/topics/T.
#PubSubTopic: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.pubsub.topic"
		identity_fields: ["name"]
		review: status: "draft"
	}
	project: string
	name:    =~"^projects/[^/]+/topics/[^/]+$"
	...
}

// `gcloud pubsub subscriptions list --format=json`. A push endpoint URL can
// embed a token, so it is redacted.
#PubSubSubscription: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.pubsub.subscription"
		identity_fields: ["name"]
		review: status: "draft"
		sensitive_fields: ["pushConfig.pushEndpoint"]
	}
	project: string
	name:    =~"^projects/[^/]+/subscriptions/[^/]+$"
	topic:   string
	...
}

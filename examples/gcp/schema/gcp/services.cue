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
		owner: _#GCPOwner & {paths: ["metadata.labels.team"]}
		sensitive_fields: [
			"spec.template.spec.containers[*].env[*].value",
			"metadata.annotations.\"serving.knative.dev/creator\"",
			"metadata.annotations.\"serving.knative.dev/lastModifier\"",
		]
	}
	apiVersion: "serving.knative.dev/v1" @pudl(match)
	kind:       "Service" @pudl(match)
	project:    _gcpProjectID
	metadata: {
		name:      string
		namespace: string
		// Services carry other labels too (Cloud Functions 2nd gen adds
		// goog-cloudfunctions-*), so labels stays open.
		labels: {
			"cloud.googleapis.com/location": string @pudl(match)
			...
		}
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
		owner: _#GCPOwner & {paths: ["labels.team"]}
		facts: {
			gcp_dns_zone: args: {project: "project", name: "name"}
			// One fact per network a private zone is visible to.
			gcp_dns_zone_network: {
				each: "privateVisibilityConfig.networks[*]"
				args: network: {path: "networkUrl", trim_prefix: _gcpComputeAPI}
			}
		}
	}
	kind:        "dns#managedZone" @pudl(match)
	project:     _gcpProjectID
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
		owner: _#GCPOwner & {paths: ["labels.team"]}
	}
	project: _gcpProjectID
	name:    =~"^projects/[^/]+/topics/[^/]+$" @pudl(match)
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
		owner: _#GCPOwner & {paths: ["labels.team"]}
		sensitive_fields: ["pushConfig.pushEndpoint"]
	}
	project: _gcpProjectID
	name:    =~"^projects/[^/]+/subscriptions/[^/]+$" @pudl(match)
	topic:   string
	...
}

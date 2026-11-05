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
		// Push subscriptions name a service by its URL.
		graph: _gcpGraph & {name: "metadata.name", region: "metadata.labels.\"cloud.googleapis.com/location\"", refs: [{path: "status.url"}]}
		graph: display: {code: "RUN", label: "Cloud Run service", class: "edge"}
		sensitive_fields: [
			"spec.template.spec.containers[*].env[*].value",
			"metadata.annotations.\"serving.knative.dev/creator\"",
			"metadata.annotations.\"serving.knative.dev/lastModifier\"",
		]
		// The service account revisions run as; absent means the Compute
		// Engine default account.
		facts: gcp_run_service_sa: args: email: "spec.template.spec.serviceAccountName"
		// The Artifact Registry image each container runs, as the image path
		// gcp.#ContainerImage is named by: tag and digest dropped, gcr.io
		// hosts mapped to the Artifact Registry repositories that serve
		// them. Images from elsewhere (Docker Hub and so on) yield no fact.
		facts: gcp_run_service_image: args: image: {
			path: "spec.template.spec.containers[*].image"
			rewrite: [
				{match: "^([a-z0-9-]+-docker\\.pkg\\.dev/[^:@]+)", replace: "$1"},
				{match: "^gcr\\.io/([^/]+)/([^:@]+)", replace: "us-docker.pkg.dev/${1}/gcr.io/${2}"},
				{match: "^us\\.gcr\\.io/([^/]+)/([^:@]+)", replace: "us-docker.pkg.dev/${1}/us.gcr.io/${2}"},
				{match: "^eu\\.gcr\\.io/([^/]+)/([^:@]+)", replace: "europe-docker.pkg.dev/${1}/eu.gcr.io/${2}"},
				{match: "^asia\\.gcr\\.io/([^/]+)/([^:@]+)", replace: "asia-docker.pkg.dev/${1}/asia.gcr.io/${2}"},
			]
		}
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
		graph: _gcpGraph
		graph: display: {code: "DNS", label: "DNS zone", class: "edge"}
		graph: network: {}
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

// `gcloud dns record-sets list --zone=Z` for every zone of a project, with
// the zone's name added (managedZone; gcloud prints none). A record set is
// unique by zone, name and type. Records are what the zone publishes (to
// the internet, or to the networks a private zone is visible to), so none
// is redacted.
#DNSRecordSet: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.dns.record_set"
		identity_fields: ["project", "managedZone", "name", "type"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: "name"}
		graph: display: {code: "RR", label: "DNS record set", class: "edge"}
		graph: inventory: collapsed: true
		facts: gcp_dns_record: args: {
			project: "project"
			zone:    "managedZone"
			name:    "name"
			type:    "type"
			data:    "rrdatas[*]"
		}
	}
	kind:        "dns#resourceRecordSet" @pudl(match)
	project:     _gcpProjectID
	managedZone: string
	name:        string
	type:        string
	ttl?:        int
	rrdatas?: [...string]
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
		graph: _gcpGraph & {name: {path: "name", last_segment: true}}
		graph: display: {code: "TOPIC", label: "Pub/Sub topic", class: "messaging"}
		// The customer-managed key messages are encrypted with, if any.
		// (A rewrite, so a topic without one yields no fact.)
		facts: gcp_topic_key: args: key: {path: "kmsKeyName", rewrite: [{match: "^(projects/.+/cryptoKeys/[^/]+)$", replace: "$1"}]}
	}
	project: _gcpProjectID
	name:    =~"^projects/[^/]+/topics/[^/]+$" @pudl(match)
	...
}

// `gcloud pubsub subscriptions list --format=json`. A push endpoint URL can
// embed a token, so it is redacted; the collector's transform keeps its
// origin (scheme and host, no credentials, path or query) as
// pushConfig.pushEndpointOrigin, which is how a push subscription names the
// Cloud Run service it delivers to.
#PubSubSubscription: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.pubsub.subscription"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpGraph & {name: {path: "name", last_segment: true}}
		graph: display: {code: "SUB", label: "Pub/Sub subscription", class: "messaging"}
		facts: gcp_subscription_topic: args: topic: "topic"
		// Where undeliverable messages are forwarded, if anywhere.
		// (A rewrite, so a subscription without one yields no fact.)
		facts: gcp_subscription_dead_letter: args: topic: {path: "deadLetterPolicy.deadLetterTopic", rewrite: [{match: "^(projects/[^/]+/topics/[^/]+)$", replace: "$1"}]}
		// Where a push subscription delivers, by origin; pull subscriptions
		// yield no fact.
		facts: gcp_subscription_push: args: endpoint: {path: "pushConfig.pushEndpointOrigin", rewrite: [{match: "^(https?://.+)$", replace: "$1"}]}
		sensitive_fields: ["pushConfig.pushEndpoint"]
	}
	project: _gcpProjectID
	name:    =~"^projects/[^/]+/subscriptions/[^/]+$" @pudl(match)
	topic:   string
	...
}

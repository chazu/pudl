package models

import (
	"list"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// GCP content pack models (README: examples/gcp/README.md in the PUDL source).
//
// One collector per resource type, in two variants:
//   gcp-<type>          runs gcloud against every project gcloud lists;
//   gcp-fixture-<type>  reads the bundled fixtures in .pudl/populators/gcp.
// Three checks-only families evaluate the latest collector observations:
//   gcp[-fixture]-network-hygiene-checks, -data-protection-checks and
//   -gke-posture-checks.
// Run a family together with its collectors, which it depends on, e.g.
//   pudl check gcp-firewalls gcp-addresses gcp-routers gcp-network-hygiene-checks

// ---- Live settings. Each has a default; to change one without editing this
// file, set it in another file of package models, e.g.
// .pudl/schema/models/gcp_settings.cue: _gcpProjectFilter: "labels.env=staging"

// The live collectors fan out over every project `gcloud projects list` prints
// for your credentials, discovered at run time. A gcloud filter narrows the
// list, e.g. "labels.env=staging" or "projectId~staging" (a regex match);
// "" lists them all.
_gcpProjectFilter: string | *""

// Locations gcp-kms-keyrings and gcp-kms-keys list key rings in, per project.
_gcpKMSLocations: [...string] | *["global"]

// ---- End of live settings. ----

// The schema each collector routes its records to.
_gcpSchemas: {
	networks:               "gcp.#Network"
	subnets:                "gcp.#Subnetwork"
	firewalls:              "gcp.#Firewall"
	routes:                 "gcp.#Route"
	routers:                "gcp.#Router"
	addresses:              "gcp.#Address"
	"gke-clusters":         "gcp.#GKECluster"
	"sql-instances":        "gcp.#SQLInstance"
	buckets:                "gcp.#Bucket"
	"run-services":         "gcp.#RunService"
	"dns-zones":            "gcp.#DNSManagedZone"
	"pubsub-topics":        "gcp.#PubSubTopic"
	"pubsub-subscriptions": "gcp.#PubSubSubscription"
	"kms-keyrings":         "gcp.#KMSKeyRing"
	"kms-keys":             "gcp.#KMSKey"
}

// The gcloud list command of each per-project collector. Each prints every
// resource of its type in the project (all regions and zones).
_gcpCommands: {
	networks: ["compute", "networks", "list"]
	subnets: ["compute", "networks", "subnets", "list"]
	firewalls: ["compute", "firewall-rules", "list"]
	routes: ["compute", "routes", "list"]
	routers: ["compute", "routers", "list"]
	addresses: ["compute", "addresses", "list"]
	"gke-clusters": ["container", "clusters", "list"]
	"sql-instances": ["sql", "instances", "list"]
	buckets: ["storage", "buckets", "list"]
	"run-services": ["run", "services", "list"]
	"dns-zones": ["dns", "managed-zones", "list"]
	"pubsub-topics": ["pubsub", "topics", "list"]
	"pubsub-subscriptions": ["pubsub", "subscriptions", "list"]
}

// The enumeration every live collector repeats its runs over: one input per
// project ID.
_gcpProjectList: {
	argv: list.Concat([
		["gcloud", "projects", "list", "--format=json"],
		[if _gcpProjectFilter != "" {"--filter=\(_gcpProjectFilter)"}],
	])
	select: "projectId"
}

// Live runs, repeated per enumerated project ({{item}}): one per type, one per
// location for key rings, and a helper that lists each ring's keys. gcloud
// omits the project, so every run sets it.
_gcpRuns: {
	for t, args in _gcpCommands {
		(t): [{
			argv: list.Concat([["gcloud"], args, ["--project={{item}}", "--format=json"]])
			set: project: "{{item}}"
		}]
	}
	"kms-keyrings": [for l in _gcpKMSLocations {
		argv: ["gcloud", "kms", "keyrings", "list", "--location=\(l)", "--project={{item}}", "--format=json"]
		set: project: "{{item}}"
	}]
	"kms-keys": [{
		argv: list.Concat([["sh", "../../populators/gcp/kms-keys.sh", "{{item}}"], _gcpKMSLocations])
		set: project: "{{item}}"
	}]
}

// A collector observes one type completely. A run that fails is a failed
// input: the other runs are ingested and the observation is incomplete.
// A live run whose API is disabled in the project is an empty input instead:
// the project has none of the type, so the observation stays complete and the
// report lists the project under empty_inputs. gcloud words this per API:
// most print the reason SERVICE_DISABLED; some (container, kms) print only
// "... API has not been used in project P before or it is disabled". Any
// other failure, including a project with billing disabled, is still a
// failed input.
_#GCPCollector: {
	_type:   string
	_prefix: string
	_scope:  string
	_runs: [...]
	sm.#SystemModel
	name: "\(_prefix)\(_type)"
	observation: {scope: "\(_scope)/\(_type)", complete: true}
	populate: sm.#CommandObserve & {schema: _gcpSchemas[_type], runs: _runs}
}

_#GCPLive: _#GCPCollector & {
	_type:   string
	_prefix: "gcp-"
	_scope:  "gcp"
	_runs:   _gcpRuns[_type]
	populate: {
		each: _gcpProjectList
		empty_when: [
			"SERVICE_DISABLED",
			"API has not been used in project [^ ]+ before or it is disabled",
		]
	}
}

_#GCPFixture: _#GCPCollector & {
	_type:   string
	_prefix: "gcp-fixture-"
	_scope:  "fixture/gcp"
	_runs: [{argv: ["cat", "../../populators/gcp/\(_type).json"], set: project: "example-project"}]
}

#GCPNetworks: _#GCPLive & {_type: "networks"}
#GCPSubnets: _#GCPLive & {_type: "subnets"}
#GCPFirewalls: _#GCPLive & {_type: "firewalls"}
#GCPRoutes: _#GCPLive & {_type: "routes"}
#GCPRouters: _#GCPLive & {_type: "routers"}
#GCPAddresses: _#GCPLive & {_type: "addresses"}
#GCPGKEClusters: _#GCPLive & {_type: "gke-clusters"}
#GCPSQLInstances: _#GCPLive & {_type: "sql-instances"}
#GCPBuckets: _#GCPLive & {_type: "buckets"}
#GCPRunServices: _#GCPLive & {_type: "run-services"}
#GCPDNSZones: _#GCPLive & {_type: "dns-zones"}
#GCPPubSubTopics: _#GCPLive & {_type: "pubsub-topics"}
#GCPPubSubSubscriptions: _#GCPLive & {_type: "pubsub-subscriptions"}
#GCPKMSKeyRings: _#GCPLive & {_type: "kms-keyrings"}
#GCPKMSKeys: _#GCPLive & {_type: "kms-keys"}
#GCPFixtureNetworks: _#GCPFixture & {_type: "networks"}
#GCPFixtureSubnets: _#GCPFixture & {_type: "subnets"}
#GCPFixtureFirewalls: _#GCPFixture & {_type: "firewalls"}
#GCPFixtureRoutes: _#GCPFixture & {_type: "routes"}
#GCPFixtureRouters: _#GCPFixture & {_type: "routers"}
#GCPFixtureAddresses: _#GCPFixture & {_type: "addresses"}
#GCPFixtureGKEClusters: _#GCPFixture & {_type: "gke-clusters"}
#GCPFixtureSQLInstances: _#GCPFixture & {_type: "sql-instances"}
#GCPFixtureBuckets: _#GCPFixture & {_type: "buckets"}
#GCPFixtureRunServices: _#GCPFixture & {_type: "run-services"}
#GCPFixtureDNSZones: _#GCPFixture & {_type: "dns-zones"}
#GCPFixturePubSubTopics: _#GCPFixture & {_type: "pubsub-topics"}
#GCPFixturePubSubSubscriptions: _#GCPFixture & {_type: "pubsub-subscriptions"}
#GCPFixtureKMSKeyRings: _#GCPFixture & {_type: "kms-keyrings"}
#GCPFixtureKMSKeys: _#GCPFixture & {_type: "kms-keys"}

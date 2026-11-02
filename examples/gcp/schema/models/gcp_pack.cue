package models

import (
	"list"
	"strings"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// GCP content pack models (README: examples/gcp/README.md in the PUDL source).
//
// One collector per resource type, in two variants:
//   gcp-<type>          runs gcloud against the projects below;
//   gcp-fixture-<type>  reads the bundled fixtures in .pudl/populators/gcp.
// Three checks-only families evaluate the latest collector observations:
//   gcp[-fixture]-network-hygiene-checks, -data-protection-checks and
//   -gke-posture-checks.
// Run a family together with its collectors, which it depends on, e.g.
//   pudl check gcp-firewalls gcp-addresses gcp-routers gcp-network-hygiene-checks

// ---- Live settings: replace these before running the gcp-* models. ----

// Project IDs every live collector fans out over.
_gcpProjects: ["REPLACE_WITH_YOUR_PROJECT"]

// Locations gcp-kms-keyrings lists in each project.
_gcpKMSLocations: ["global"]

// Fully qualified key rings gcp-kms-keys lists keys from.
_gcpKMSKeyRings: ["projects/REPLACE_WITH_YOUR_PROJECT/locations/global/keyRings/REPLACE_WITH_YOUR_KEYRING"]

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

// Live runs: projects for most types, projects x locations for key rings, and
// the listed key rings for keys. gcloud omits the project, so every run sets it.
_gcpRuns: {
	for t, args in _gcpCommands {
		(t): [for p in _gcpProjects {
			argv: list.Concat([["gcloud"], args, ["--project=\(p)", "--format=json"]])
			set: project: p
		}]
	}
	"kms-keyrings": [for p in _gcpProjects for l in _gcpKMSLocations {
		argv: ["gcloud", "kms", "keyrings", "list", "--location=\(l)", "--project=\(p)", "--format=json"]
		set: project: p
	}]
	"kms-keys": [for k in _gcpKMSKeyRings {
		argv: ["gcloud", "kms", "keys", "list", "--keyring=\(k)", "--format=json"]
		set: project: strings.Split(k, "/")[1]
	}]
}

// A collector observes one type completely. A run that fails is a failed
// input: the other runs are ingested and the observation is incomplete.
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

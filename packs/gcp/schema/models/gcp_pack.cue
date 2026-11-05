package models

import (
	"list"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// GCP content pack models (README: packs/gcp/README.md in the PUDL source).
//
// One collector per resource type, in two variants:
//   gcp-<type>          runs gcloud against every project gcloud lists, or
//                       for the types in _gcpAssetTypes (gcp_assets.cue)
//                       searches Cloud Asset Inventory first;
//   gcp-fixture-<type>  reads the bundled fixtures in .pudl/populators/gcp
//                       (a CAI search result under cai/ first, likewise).
// Four checks-only families evaluate the latest collector observations:
//   gcp[-fixture]-network-hygiene-checks, -data-protection-checks,
//   -gke-posture-checks and -iam-hygiene-checks.
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

// More errors to treat as "this project has none" on top of the disabled-API
// patterns every live collector matches (_gcpEmptyWhen). Only add an error
// you have decided means no resources for you, e.g. "Billing is disabled for
// project" when unbilled projects' resources do not count.
_gcpExtraEmptyWhen: [...string] | *[]

// Where the collectors of the types in _gcpAssetTypes (gcp_assets.cue) search
// Cloud Asset Inventory: "organization" searches every organization the
// credentials can see, discovered at run time, once per type; "projects"
// searches each project gcloud lists (_gcpProjectFilter applies); "off" uses
// only the gcloud list commands. Whenever CAI cannot answer (the asset API is
// disabled or unbillable, or the caller may not search), a collector falls
// back to its gcloud commands and the report says so.
_gcpAssetScope: *"organization" | "projects" | "off"

// The project CAI calls are billed to (--billing-project). It needs
// cloudasset.googleapis.com enabled and serviceusage.services.use for the
// caller. Empty uses gcloud's own billing/quota_project setting.
_gcpBillingProject: string | *""

// How many tagged versions gcp-container-images keeps per image, newest push
// first.
_gcpImageHistory: int & >0 | *10

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
	instances:              "gcp.#ComputeInstance"
	"service-accounts":     "gcp.#ServiceAccount"
	"iam-policies":         "gcp.#IAMPolicy"
	"iam-roles":            "gcp.#IAMRole"
	"service-account-keys": "gcp.#ServiceAccountKey"
	"container-images":     "gcp.#ContainerImage"
	"k8s-deployments":      "gcp.#K8sDeployment"
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
	instances: ["compute", "instances", "list"]
	"service-accounts": ["iam", "service-accounts", "list"]
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
	// get-iam-policy takes the project as an argument and prints one object,
	// the project's own policy (gcp_iam.cue).
	"iam-policies": [_gcpProjectPolicyRun & {
		_p: "{{item}}"
		argv: ["gcloud", "projects", "get-iam-policy", "{{item}}", "--format=json"]
	}]
	// Helpers: list the project's custom roles, then describe each (the list
	// omits permissions); list its service accounts, then each one's
	// user-managed keys.
	"iam-roles": [{
		argv: ["sh", "../../populators/gcp/iam-roles.sh", "{{item}}"]
		set: project: "{{item}}"
	}]
	"service-account-keys": [{
		argv: ["sh", "../../populators/gcp/service-account-keys.sh", "{{item}}"]
		set: project: "{{item}}"
	}]
	// Helper: list the project's Docker repositories, their images, then
	// each image's newest tagged versions (gcp_images.cue).
	"container-images": [{
		argv: ["sh", "../../populators/gcp/container-images.sh", "{{item}}", "\(_gcpImageHistory)"]
		set: project: "{{item}}"
	}]
	// CAI-only types (_gcpCAIOnly) have no gcloud runs.
	for t, _ in _gcpCAIOnly {(t): []}
}

// Run timeouts of collectors whose runs can outlast the default (10m).
_gcpRunTimeout: [string]: string

// A collector observes one type completely. A run that fails is a failed
// input: the other runs are ingested and the observation is incomplete.
// A live run whose API is disabled in the project is an empty input instead:
// the project has none of the type, so the observation stays complete and the
// report lists the project under empty_inputs. gcloud words this per API:
// most print the reason SERVICE_DISABLED; some (container, kms) print only
// "... API has not been used in project P before or it is disabled". Any
// other failure, including a project with billing disabled, is still a
// failed input unless _gcpExtraEmptyWhen names it.
_gcpEmptyWhen: [
	"SERVICE_DISABLED",
	"API has not been used in project [^ ]+ before or it is disabled",
]

// A collector's _gcloud source is its gcloud list commands; a type in
// _gcpAssetTypes, and gcp-iam-policies (gcp_iam.cue), searches CAI first and
// falls back to them. A gcloud source in _gcpPartialGcloud observes only
// part of the type, incompletely. A type in _gcpCAIOnly searches CAI only.
_#GCPCollector: {
	_type:   string
	_prefix: string
	_scope:  string
	_gcloud: {...}
	_cai: {...}
	_iamPolicies: _type == "iam-policies"
	_caiOnly:     _gcpCAIOnly[_type] != _|_
	_useCAI:      _caiOnly || (_gcpAssetScope != "off" && (_gcpAssetTypes[_type] != _|_ || _iamPolicies))
	_partial:     _gcpPartialGcloud[_type] != _|_
	sm.#SystemModel
	name: "\(_prefix)\(_type)"
	observation: {scope: "\(_scope)/\(_type)", complete: _useCAI || !_partial}
	populate: sm.#CommandObserve & {schema: _gcpSchemas[_type]}
	if !_useCAI {
		populate: _gcloud
	}
	if _caiOnly {
		populate: _cai
	}
	if _useCAI && !_caiOnly {
		populate: _cai & {fallback: {
			when: [if _iamPolicies {_gcpIAMSearchUnavailable}, _gcpAssetUnavailable][0]
			if _partial {
				partial: true
			}
			observe: _gcloud
		}}
	}
}

// The gcloud runs of a type, put into its shape when it has one.
_gcpShapedRuns: {
	_t: string
	let type = _t
	_runs: [...]
	out: [for r in _runs {
		r
		if _gcpShapes[type] != _|_ {
			transform: _gcpGcloudTransform & {_t: type}
		}
	}]
}

_#GCPLive: _#GCPCollector & {
	_type:   string
	_prefix: "gcp-"
	_scope:  "gcp"
	// Types above projects (gcp_hierarchy.cue) run per organization, where
	// no error means "none of the type".
	_orgLevel: _gcpOrgLevel[_type] != _|_
	_gcloud: {
		each: [if _orgLevel {_gcpOrgList}, _gcpProjectList][0]
		empty_when: [if _orgLevel {[]}, list.Concat([_gcpEmptyWhen, _gcpExtraEmptyWhen])][0]
		if _gcpRunTimeout[_type] != _|_ {
			timeout: _gcpRunTimeout[_type]
		}
		runs: (_gcpShapedRuns & {_t: _type, _runs: _gcpRuns[_type]}).out
	}
	_cai: [if _type == "iam-policies" {_gcpIAMPolicySearch}, _gcpAssetSearch & {_t: _type}][0]
}

// Fixtures: the bundled gcloud output, and for CAI types a bundled CAI search
// result (populators/gcp/cai/) read first, as live.
_#GCPFixture: _#GCPCollector & {
	_type:   string
	_prefix: "gcp-fixture-"
	_scope:  "fixture/gcp"
	_gcloud: runs: (_gcpShapedRuns & {_t: _type, _runs: [
		[if _gcpFixtureGcloudRun[_type] != _|_ {_gcpFixtureGcloudRun[_type]}, {}][0] & {
			argv: ["cat", "../../populators/gcp/\(_type).json"]
			if _gcpOrgLevel[_type] == _|_ {
				set: project: "example-project"
			}
		},
	]}).out
	_cai: runs: [{
		argv: ["cat", "../../populators/gcp/cai/\(_type).json"]
		if _type == "iam-policies" || _gcpAssetByProject[_type] != _|_ {
			set: project: "example-project"
		}
		if _type == "iam-policies" {
			transform: _gcpIAMPolicyTransform
		}
		if _type != "iam-policies" {
			transform: _gcpAssetTransform & {_t: _type}
		}
	}]
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
#GCPInstances: _#GCPLive & {_type: "instances"}
#GCPServiceAccounts: _#GCPLive & {_type: "service-accounts"}
#GCPIAMPolicies: _#GCPLive & {_type: "iam-policies"}
#GCPIAMRoles: _#GCPLive & {_type: "iam-roles"}
#GCPServiceAccountKeys: _#GCPLive & {_type: "service-account-keys"}
#GCPContainerImages: _#GCPLive & {_type: "container-images"}
#GCPK8sDeployments: _#GCPLive & {_type: "k8s-deployments"}
#GCPOrganizations: _#GCPLive & {_type: "organizations"}
#GCPFolders: _#GCPLive & {_type: "folders"}
#GCPOrgPolicies: _#GCPLive & {_type: "org-policies"}
#GCPFirewallPolicies: _#GCPLive & {_type: "firewall-policies"}
#GCPBinauthzPolicies: _#GCPLive & {_type: "binauthz-policies"}
#GCPServices: _#GCPLive & {_type: "services"}
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
#GCPFixtureInstances: _#GCPFixture & {_type: "instances"}
#GCPFixtureServiceAccounts: _#GCPFixture & {_type: "service-accounts"}
#GCPFixtureIAMPolicies: _#GCPFixture & {_type: "iam-policies"}
#GCPFixtureIAMRoles: _#GCPFixture & {_type: "iam-roles"}
#GCPFixtureServiceAccountKeys: _#GCPFixture & {_type: "service-account-keys"}
#GCPFixtureContainerImages: _#GCPFixture & {_type: "container-images"}
#GCPFixtureK8sDeployments: _#GCPFixture & {_type: "k8s-deployments"}
#GCPFixtureOrganizations: _#GCPFixture & {_type: "organizations"}
#GCPFixtureFolders: _#GCPFixture & {_type: "folders"}
#GCPFixtureOrgPolicies: _#GCPFixture & {_type: "org-policies"}
#GCPFixtureFirewallPolicies: _#GCPFixture & {_type: "firewall-policies"}
#GCPFixtureBinauthzPolicies: _#GCPFixture & {_type: "binauthz-policies"}
#GCPFixtureServices: _#GCPFixture & {_type: "services"}

package models

import (
	"list"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// GCP content pack check families. Each family is checks-only: it reads the
// latest observation of each collector it names (evidence "model:<name>"),
// which must be at most an hour old. Policy results come from the policies in
// schema/gcp/policies.cue; the cross-resource checks use rules in
// schema/pudl/rules/gcp_pack.cue.

// A check over the latest observations of the named collectors.
_#GCPCheck: {
	_prefix: string
	_from: [...string]
	sm.#Check
	evidence: [for c in _from {"model:\(_prefix)\(c)"}]
	max_age: "1h"
	expect:  "empty"
}

// Fail on any high or critical policy violation; warn on any violation.
_#GCPPolicyChecks: {
	_prefix: string
	_from: [...string]
	_what: string
	let prefix = _prefix
	let from = _from
	out: [
		_#GCPCheck & {
			_prefix:  prefix, _from: from
			name:     "policy-high"
			query:    "gcp_policy_noncompliant_high"
			severity: "fail"
			message:  "Resolve high and critical policy violations on \(_what) (pudl policy evaluate --json lists them)"
		},
		_#GCPCheck & {
			_prefix:  prefix, _from: from
			name:     "policy-any"
			query:    "policy_noncompliant"
			severity: "warn"
			message:  "Review every policy violation on \(_what)"
		},
	]
}

// A family: its name, the collectors it runs after, and its checks.
_#GCPFamily: {
	_prefix: string
	_family: string
	_collectors: [...string]
	sm.#SystemModel
	name: "\(_prefix)\(_family)-checks"
	depends_on: [for c in _collectors {"\(_prefix)\(c)"}]
}

_#GCPNetworkHygiene: _#GCPFamily & {
	_prefix: string
	let P = _prefix
	_family: "network-hygiene"
	_collectors: ["firewalls", "addresses", "routers"]
	checks: list.Concat([(_#GCPPolicyChecks & {_prefix: P, _from: ["firewalls", "addresses"], _what: "firewalls and addresses"}).out, [
		_#GCPCheck & {
			_prefix: P, _from: ["routers", "addresses"]
			name:     "nat-ip-not-in-use"
			query:    "gcp_nat_ip_not_in_use"
			severity: "fail"
			message:  "A Cloud NAT gateway lists an external address that GCP reports as RESERVED (not in use)"
		},
	]])
}

_#GCPDataProtection: _#GCPFamily & {
	_prefix: string
	let P = _prefix
	_family: "data-protection"
	_collectors: ["buckets", "sql-instances", "kms-keys"]
	checks: (_#GCPPolicyChecks & {_prefix: P, _from: ["buckets", "sql-instances", "kms-keys"], _what: "buckets, Cloud SQL instances and KMS keys"}).out
}

_#GCPGKEPosture: _#GCPFamily & {
	_prefix: string
	let P = _prefix
	_family: "gke-posture"
	_collectors: ["gke-clusters", "subnets", "kms-keys"]
	checks: list.Concat([(_#GCPPolicyChecks & {_prefix: P, _from: ["gke-clusters"], _what: "GKE clusters"}).out, [
		_#GCPCheck & {
			_prefix: P, _from: ["gke-clusters", "subnets"]
			name:     "private-nodes-without-private-google-access"
			query:    "gcp_gke_subnet_without_private_access"
			severity: "fail"
			message:  "Enable Private Google Access on the subnet of each cluster with private nodes"
		},
		_#GCPCheck & {
			_prefix: P, _from: ["gke-clusters", "kms-keys"]
			name:     "secrets-key-unusable"
			query:    "gcp_gke_secrets_key_unusable"
			severity: "fail"
			message:  "A cluster encrypts secrets with a KMS key whose primary version is disabled or destroyed"
		},
	]])
}

// User-managed service account keys: rotation and expiry.
_#GCPIAMHygiene: _#GCPFamily & {
	_prefix: string
	let P = _prefix
	_family: "iam-hygiene"
	_collectors: ["service-account-keys"]
	checks: [
		_#GCPCheck & {
			_prefix: P, _from: ["service-account-keys"]
			name:     "sa-key-older-than-90d"
			query:    "gcp_sa_key_older_than_90d"
			severity: "fail"
			message:  "Rotate user-managed service account keys created more than 90 days ago"
		},
		_#GCPCheck & {
			_prefix: P, _from: ["service-account-keys"]
			name:     "sa-key-never-expires"
			query:    "gcp_sa_key_never_expires"
			severity: "warn"
			message:  "Give user-managed service account keys an expiry (constraints/iam.serviceAccountKeyExpiryHours)"
		},
	]
}

#GCPNetworkHygieneChecks: _#GCPNetworkHygiene & {_prefix: "gcp-"}
#GCPDataProtectionChecks: _#GCPDataProtection & {_prefix: "gcp-"}
#GCPGKEPostureChecks: _#GCPGKEPosture & {_prefix: "gcp-"}
#GCPFixtureNetworkHygieneChecks: _#GCPNetworkHygiene & {_prefix: "gcp-fixture-"}
#GCPFixtureDataProtectionChecks: _#GCPDataProtection & {_prefix: "gcp-fixture-"}
#GCPFixtureGKEPostureChecks: _#GCPGKEPosture & {_prefix: "gcp-fixture-"}
#GCPIAMHygieneChecks: _#GCPIAMHygiene & {_prefix: "gcp-"}
#GCPFixtureIAMHygieneChecks: _#GCPIAMHygiene & {_prefix: "gcp-fixture-"}

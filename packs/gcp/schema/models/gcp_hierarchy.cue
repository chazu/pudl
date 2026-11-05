package models

import "regexp"

// Collectors of the resource hierarchy (schema/gcp/hierarchy.cue):
// organizations, folders, organization policies and hierarchical firewall
// policies. Their resources live above projects, so:
//   - the CAI search runs once per organization the credentials can see,
//     whatever _gcpAssetScope says (a project search never finds them), and
//     keeps resources outside projects;
//   - the gcloud commands run once per organization ({{item}} is
//     organizations/N), walking its folders through populators/gcp/folders.sh,
//     and set no project;
//   - no failure counts as "this organization has none": a disabled API
//     error names gcloud's quota project, not the organization.
//
// Live parity (2026-11-05): organizations and folders are identical in both
// sources. Organization policies and firewall policies could not be listed
// with gcloud by the surveying account (no Org Policy API in its quota
// project, no compute.firewallPolicies.list at the organization); their
// gcloud shapes follow the API documentation and are unverified.
_gcpOrgLevel: {
	organizations:       true
	folders:             true
	"org-policies":      true
	"firewall-policies": true
}

_gcpAssetTypes: {
	organizations:       "cloudresourcemanager.googleapis.com/Organization"
	folders:             "cloudresourcemanager.googleapis.com/Folder"
	"org-policies":      "orgpolicy.googleapis.com/Policy"
	"firewall-policies": "compute.googleapis.com/FirewallPolicy"
}

_gcpSchemas: {
	organizations:       "gcp.#Organization"
	folders:             "gcp.#Folder"
	"org-policies":      "gcp.#OrgPolicy"
	"firewall-policies": "gcp.#FirewallPolicy"
}

_gcpRuns: {
	organizations: [{argv: ["gcloud", "organizations", "describe", "{{item}}", "--format=json"]}]
	// Helpers: every folder of the organization; every policy set on it,
	// its folders and their projects, described; every firewall policy of
	// it and its folders.
	folders: [{argv: ["sh", "../../populators/gcp/folders.sh", "{{item}}"]}]
	"org-policies": [{argv: ["sh", "../../populators/gcp/org-policies.sh", "{{item}}"]}]
	"firewall-policies": [{argv: ["sh", "../../populators/gcp/firewall-policies.sh", "{{item}}"]}]
}

_gcpShapes: {
	organizations: _gcpSameShape
	folders:       _gcpSameShape
	// etag is an optimistic-lock token (CAI prints it on the policy, the API
	// also on its spec).
	"org-policies": {
		in: {...}
		let r = in
		out: {
			for k, v in r if k != "etag" && k != "spec" {(k): v}
			if r.spec != _|_ {
				spec: {for k, v in r.spec if k != "etag" {(k): v}}
			}
		}
	}
	// CAI omits kind; fingerprint is a lock token.
	"firewall-policies": _gcpComputeShape & {_kind: "compute#firewallPolicy"}
}

// CAI lists network firewall policies (a project's) under the same asset
// type; only hierarchical ones (locations/global, set on an organization or
// folder) are collected.
_gcpSkip: "firewall-policies": {
	in: {...}
	let r = in
	skip: !regexp.Match("/locations/global/firewallPolicies/[^/]+$", r.selfLink)
}

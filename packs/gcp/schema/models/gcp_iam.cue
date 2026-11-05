package models

import "list"

// IAM sources of the GCP pack: allow policies on every resource
// (gcp-iam-policies), custom roles (gcp-iam-roles) and user-managed service
// account keys (gcp-service-account-keys). Roles and keys are ordinary
// Cloud Asset Inventory types (gcp_assets.cue); policies have their own
// search, below.
//
// Policies are searched per project whatever _gcpAssetScope says: the
// permission to search them (cloudasset.assets.searchAllIamPolicies) is
// often granted on some projects and not at the organization. A project
// that refuses the search is a failed input, not a reason to fall back:
// gcloud's projects get-iam-policy needs a similar grant and is refused
// in the same projects. Only an unusable billing project switches to gcloud,
// which reads each project's own policy and not its resources', so that
// fallback is partial.

// Errors that mean CAI cannot search IAM policies at all: the asset API is
// disabled in the billing project, or the caller may not bill it.
_gcpIAMSearchUnavailable: ["SERVICE_DISABLED", "USER_PROJECT_DENIED"]

// Collectors whose gcloud source sees only some of the type's resources: its
// observation (as fallback, or with _gcpAssetScope: "off") is incomplete, so
// it marks nothing gone.
_gcpPartialGcloud: {"iam-policies": true}

// The CAI populate of gcp-iam-policies: one search per project.
_gcpIAMPolicySearch: {
	each: _gcpProjectList
	runs: [{
		argv: list.Concat([
			["gcloud", "asset", "search-all-iam-policies", "--scope=projects/{{item}}", "--format=json"],
			[if _gcpBillingProject != "" {"--billing-project=\(_gcpBillingProject)"}],
		])
		set: project: "{{item}}"
		transform: _gcpIAMPolicyTransform
	}]
}

// A search result in gcp.#IAMPolicy's shape. The search names the project by
// number and lists the resource's folders and organization, which gcloud's
// fallback cannot supply; the run sets the project ID instead.
_gcpIAMPolicyTransform: {
	in: _
	out: {
		resource:  in.resource
		assetType: in.assetType
		policy: {
			if in.policy.bindings != _|_ {
				bindings: in.policy.bindings
			}
		}
	}
}

// gcloud projects get-iam-policy prints a project's policy without naming
// the project; the run names it as the search does. Audit configs, etag and
// version are dropped (the search omits them).
_gcpProjectPolicyRun: {
	_p: string
	set: {
		project:   _p
		resource:  "//cloudresourcemanager.googleapis.com/projects/\(_p)"
		assetType: "cloudresourcemanager.googleapis.com/Project"
	}
	transform: {
		in: _
		out: policy: {
			if in.bindings != _|_ {
				bindings: in.bindings
			}
		}
	}
}

// What a fixture's gcloud run adds beyond reading its file, per type.
_gcpFixtureGcloudRun: {
	"iam-policies": _gcpProjectPolicyRun & {_p: "example-project"}
}

// Records a type does not keep, from either source: Google-managed service
// account keys (rotated by Google, never downloadable). skip is true to drop
// the record. Both sources always print keyType. (A skip guarded by an
// existence test, [if r.keyType != _|_ {...}, false][0], is never true in a
// loaded model's transform; compare directly.)
_gcpSkip: {
	"service-account-keys": {
		in: {...}
		let r = in
		skip: r.keyType == "SYSTEM_MANAGED"
	}
}

// Custom roles: the role shape of both sources. gcloud's describe adds etag;
// the search adds groupName and groupTitle ("custom") and omits stage when
// it is ALPHA, the default.
_gcpRoleShape: {
	in: {...}
	let r = in
	out: {
		for k, v in r if !list.Contains(["etag", "groupName", "groupTitle", "stage"], k) {(k): v}
		stage: [if r.stage != _|_ {r.stage}, "ALPHA"][0]
	}
}

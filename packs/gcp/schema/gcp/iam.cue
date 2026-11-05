package gcp

// `gcloud iam service-accounts list --format=json`. Identity is the email.
// IAM bindings name a service account as "serviceAccount:<email>", so that
// form is a graph reference too.
#ServiceAccount: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.iam.service_account"
		identity_fields: ["email"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {
			name: "email"
			// IAM bindings name an account as serviceAccount:EMAIL; some
			// API resource names use its unique ID.
			refs: [{path: "email", prepend: "serviceAccount:"}, {path: "uniqueId"}]
		}
		graph: display: {code: "SA", label: "Service account", class: "identity"}
		graph: security: control: true
		facts: gcp_service_account: args: {
			email:    "email"
			disabled: {path: "disabled", default: false}
		}
	}
	project:      _gcpProjectID
	name:         =~"^projects/[^/]+/serviceAccounts/" @pudl(match)
	email:        string
	displayName?: string
	disabled?:    bool
	...
}

// An IAM allow policy on one resource: a project, bucket, service account,
// secret, key, topic and so on. Collected by `gcloud asset
// search-all-iam-policies` per project, which finds every resource's policy;
// when Cloud Asset Inventory cannot answer, `gcloud projects get-iam-policy`
// supplies each project's own policy in the same shape. Identity is the
// resource's full name (//cloudresourcemanager.googleapis.com/projects/P for
// a project). Each binding grants a role to members ("user:", "group:",
// "serviceAccount:", "domain:", allUsers, allAuthenticatedUsers). Inherited
// (folder and organization) policies, conditions and deny policies are not
// evaluated, and audit configs are not kept (Cloud Asset Inventory omits
// them).
#IAMPolicy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.iam.policy"
		identity_fields: ["resource"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: {path: "resource", last_segment: true, prepend: "IAM policy · "}}
		graph: display: {code: "IAM", label: "IAM policy", class: "identity"}
		facts: {
			gcp_iam_policy: args: {asset_type: "assetType"}
			// The resource a policy is attached to, in the form its record
			// is named or referenced by: a bucket's name; a compute
			// resource's path; a service account's email (Cloud Asset
			// Inventory names accounts by email; a unique ID also resolves);
			// projects/P for a project; projects/... for topics,
			// subscriptions and KMS keys. Other resources (secrets and so
			// on) are not collected and yield no fact.
			gcp_iam_policy_resource: args: target: {
				path: "resource"
				rewrite: [
					{match: "^//storage\\.googleapis\\.com/(.+)$", replace: "$1"},
					{match: "^//compute\\.googleapis\\.com/(projects/.+)$", replace: "$1"},
					{match: "^//iam\\.googleapis\\.com/projects/[^/]+/serviceAccounts/([^/]+)$", replace: "$1"},
					{match: "^//cloudresourcemanager\\.googleapis\\.com/(projects/[^/]+)$", replace: "$1"},
					{match: "^//(?:pubsub|cloudkms)\\.googleapis\\.com/(projects/.+)$", replace: "$1"},
				]
			}
			gcp_iam_member: {
				each: "policy.bindings[*]"
				args: {role: "role", member: "members[*]", conditional: {exists: "condition"}}
			}
		}
	}
	project:   _gcpProjectID
	resource:  =~"^//[^/]+/" @pudl(match)
	assetType: =~"^[a-z0-9.]+\\.googleapis\\.com/[A-Za-z]+$"
	policy: {
		bindings?: [...{role: =~"^(roles|projects|organizations)/", members: [...string], ...}]
		...
	}
	...
}

// A custom IAM role defined in a project (iam.googleapis.com/Role).
// Predefined roles are not collected, and neither are custom roles defined
// in an organization: they belong to no project. Identity is the role name,
// projects/P/roles/ID, which bindings use as the role.
#IAMRole: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.iam.role"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: {path: "name", last_segment: true}}
		graph: display: {code: "ROLE", label: "Custom role", class: "identity"}
		facts: {
			gcp_iam_role: args: {name: "name", stage: "stage"}
			gcp_iam_role_permission: args: {name: "name", permission: "includedPermissions[*]"}
		}
	}
	project:              _gcpProjectID
	name:                 =~"^projects/[^/]+/roles/[^/]+$" @pudl(match)
	title?:               string
	description?:         string
	stage:                string
	includedPermissions?: [...string]
	...
}

// A user-managed service account key: metadata only, never key material.
// Google-managed (SYSTEM_MANAGED) keys are not collected; Google rotates
// them, and they cannot be downloaded. Identity is the key name,
// projects/P/serviceAccounts/EMAIL/keys/ID.
#ServiceAccountKey: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.iam.service_account_key"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner
		// Neither source prints key material; redact it should one appear.
		sensitive_fields: ["privateKeyData", "publicKeyData"]
		graph: _gcpGraph & {name: {path: "name", last_segment: true, prepend: "key · "}}
		graph: display: {code: "KEY", label: "Service account key", class: "identity"}
		// The account a key belongs to, by email (from the key name).
		facts: gcp_service_account_key_account: args: email: {
			path: "name"
			rewrite: [{match: "^projects/[^/]+/serviceAccounts/([^/]+)/keys/[^/]+$", replace: "$1"}]
		}
		facts: gcp_service_account_key: args: {
			name:         "name"
			key_type:     "keyType"
			valid_after:  "validAfterTime"
			valid_before: "validBeforeTime"
			disabled:     {path: "disabled", default: false}
		}
	}
	project:          _gcpProjectID
	name:             =~"^projects/[^/]+/serviceAccounts/[^/]+/keys/[^/]+$" @pudl(match)
	keyType:          "USER_MANAGED"
	keyAlgorithm?:    string
	keyOrigin?:       string
	validAfterTime?:  string
	validBeforeTime?: string
	disabled?:        bool
	...
}

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
			refs: [{path: "email", prepend: "serviceAccount:"}]
		}
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

// `gcloud projects get-iam-policy PROJECT --format=json`: one record per
// project (the collector sets project; gcloud does not print it). Each
// binding grants a role to members ("user:", "group:", "serviceAccount:",
// "domain:", allUsers, allAuthenticatedUsers). Inherited (folder and
// organization) policies, conditions and deny policies are not evaluated.
#ProjectIAMPolicy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.iam.project_policy"
		identity_fields: ["project"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: {name: {path: "project", prepend: "IAM policy · "}, account: "project"}
		facts: gcp_iam_binding: {
			each: "bindings[*]"
			args: {role: "role", member: "members[*]", conditional: {exists: "condition"}}
		}
	}
	project: _gcpProjectID
	bindings: [...{role: =~"^(roles|projects|organizations)/", members: [...string], ...}] @pudl(match)
	etag?:    string
	version?: int
	...
}

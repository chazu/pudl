package gcp

// The project every record names: the project ID the collector passed to
// gcloud --project, never the project number or display name. 6-30 lowercase
// letters, digits and hyphens, starting with a letter and not ending with a
// hyphen; legacy IDs may carry a domain prefix (example.com:my-project).
_gcpProjectID: =~#"^([a-z0-9][a-z0-9.-]*[a-z0-9]:)?[a-z][a-z0-9-]{4,28}[a-z0-9]$"#

// Project-wide settings. Neither type holds a secret, so neither declares
// sensitive fields.

// A service (API) enabled in a project: `gcloud services list --enabled`,
// Cloud Asset Inventory serviceusage.googleapis.com/Service. name is the
// service's DNS name (compute.googleapis.com); parent names the project by
// number. Only enabled services are listed.
#EnabledService: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.serviceusage.service"
		identity_fields: ["project", "name"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph
		graph: display: {code: "API", label: "Enabled service", class: "logical"}
		graph: inventory: collapsed: true
		facts: gcp_enabled_service: args: {project: "project", service: "name"}
	}
	project: _gcpProjectID
	name:    string
	parent:  =~"^projects/[0-9]+$" @pudl(match)
	state:   "ENABLED" @pudl(match)
	...
}

// A project's Binary Authorization policy: `gcloud container binauthz policy
// export`, Cloud Asset Inventory binaryauthorization.googleapis.com/Policy.
// One per project, so identity is the project. Only a policy that was set is
// kept (gcloud also prints the default of a project that never set one).
#BinaryAuthorizationPolicy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.binaryauthorization.policy"
		identity_fields: ["project"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: {path: "project", prepend: "Binary Authorization · "}}
		graph: display: {code: "BINZ", label: "Binary Authorization policy", class: "security"}
		graph: security: control: true
		facts: gcp_binauthz_policy: args: {
			project:          "project"
			evaluation_mode:  "defaultAdmissionRule.evaluationMode"
			enforcement_mode: "defaultAdmissionRule.enforcementMode"
			// ENABLE trusts Google-maintained system images.
			global_evaluation: {path: "globalPolicyEvaluationMode", default: "GLOBAL_POLICY_EVALUATION_MODE_UNSPECIFIED"}
		}
		// Images exempt from the policy, one fact per pattern.
		facts: gcp_binauthz_exempt_image: {
			each: "admissionWhitelistPatterns[*]"
			args: pattern: "namePattern"
		}
	}
	project: _gcpProjectID
	defaultAdmissionRule: {
		evaluationMode:  "ALWAYS_ALLOW" | "ALWAYS_DENY" | "REQUIRE_ATTESTATION" @pudl(match)
		enforcementMode: "ENFORCED_BLOCK_AND_AUDIT_LOG" | "DRYRUN_AUDIT_LOG_ONLY" @pudl(match)
		...
	}
	admissionWhitelistPatterns?: [...{namePattern: string}]
	...
}

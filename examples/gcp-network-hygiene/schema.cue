package gcphygiene

import "list"

#Firewall: {
	_pudl: {
		schema_type: "base"
		resource_type: "gcp.firewall"
		identity_fields: ["project", "name"]
	}
	kind: "compute#firewall" @pudl(match)
	// The project ID passed to gcloud --project, never the project number or
	// display name (see the gcp pack's _gcpProjectID).
	project: =~#"^([a-z0-9][a-z0-9.-]*[a-z0-9]:)?[a-z][a-z0-9-]{4,28}[a-z0-9]$"#
	name: string
	direction?: "INGRESS" | "EGRESS"
	disabled?: bool
	sourceRanges?: [...string]
	allowed?: [...{IPProtocol: string, ports?: [...string], ...}]
	...
}

// No enabled ingress ALLOW rule from 0.0.0.0/0. A firewall without direction
// counts as INGRESS and one without disabled as enabled.
#FirewallNoInternetIngress: {
	_pudl: {schema_type: "policy", base_schema: "pudl/gcphygiene.#Firewall"}
	direction?: _
	disabled?: _
	allowed?: _
	sourceRanges?: _ @pudl(rule="internet-ingress-allow", severity="high")
	if allowed != _|_ if !list.Contains([if direction != _|_ {direction}], "EGRESS") if !list.Contains([if disabled != _|_ {disabled}], true) {
		sourceRanges?: [...!="0.0.0.0/0"]
	}
}

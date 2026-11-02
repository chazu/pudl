package models

import sm "pudl.schemas/pudl/systemmodel@v0"

_checks: [{
	name: "internet-ingress-allow"
	query: "policy_noncompliant"
	expect: "empty"
	severity: "fail"
	message: "Review enabled ingress ALLOW rules with source 0.0.0.0/0"
	evidence: ["current"]
	max_age: "15m"
}]

#GCPNetworkHygiene: sm.#SystemModel & {
	name: "gcp-network-hygiene"
	observation: {scope: "fixture/example-project/firewalls", complete: true}
	populate: {
		schema: "pudl/gcphygiene.#Firewall"
		runs: [{argv: ["cat", "../../populators/gcp-network-hygiene/current.json"], set: project: "example-project"}]
	}
	checks: _checks
}

#GCPNetworkHygieneLive: sm.#SystemModel & {
	// Every project your gcloud credentials can list, discovered at run time.
	name: "gcp-network-hygiene-live"
	observation: {scope: "gcp/firewalls", complete: true}
	populate: {
		schema: "pudl/gcphygiene.#Firewall"
		each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
		runs: [{argv: ["gcloud", "compute", "firewall-rules", "list", "--project={{item}}", "--format=json"], set: project: "{{item}}"}]
	}
	checks: _checks
}

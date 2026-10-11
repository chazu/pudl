package models

import sm "pudl.schemas/pudl/systemmodel@v0"

_checks: [{
	name: "internet-ingress-allow"
	query: "gcp_internet_ingress_allow"
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
	// Set this project explicitly before running the live model.
	_project: "REPLACE_WITH_YOUR_PROJECT"
	name: "gcp-network-hygiene-live"
	observation: {scope: "gcp/\(_project)/firewalls", complete: true}
	populate: {
		schema: "pudl/gcphygiene.#Firewall"
		runs: [{argv: ["gcloud", "compute", "firewall-rules", "list", "--project=\(_project)", "--format=json"], set: project: _project}]
	}
	checks: _checks
}

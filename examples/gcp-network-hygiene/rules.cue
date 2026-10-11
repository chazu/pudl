package rules

gcp_internet_ingress_allow: {
	head: {rel: "gcp_internet_ingress_allow", args: {
		entry_id: "$E", project: "$P", name: "$N", protocol: "$Proto", port: "$Port"
	}}
	body: [
		{rel: "gcp_hygiene_firewall", args: {entry_id: "$E", project: "$P", name: "$N", direction: "INGRESS", disabled: false}},
		{rel: "gcp_hygiene_source", args: {entry_id: "$E", range: "0.0.0.0/0"}},
		{rel: "gcp_hygiene_allow", args: {entry_id: "$E", protocol: "$Proto", port: "$Port"}},
	]
}

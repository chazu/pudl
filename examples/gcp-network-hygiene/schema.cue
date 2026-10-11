package gcphygiene

#Firewall: {
	_pudl: {
		schema_type: "base"
		resource_type: "gcp.firewall"
		identity_fields: ["project", "name"]
		facts: {
			gcp_hygiene_firewall: args: {
				project: "project"
				name: "name"
				direction: {path: "direction", default: "INGRESS"}
				disabled: {path: "disabled", default: false}
			}
			gcp_hygiene_source: args: {range: "sourceRanges[*]"}
			gcp_hygiene_allow: {each: "allowed[*]", args: {
				protocol: "IPProtocol"
				port: {path: "ports[*]", default: "*"}
			}}
		}
	}
	kind: "compute#firewall"
	project: string
	name: string
	direction?: "INGRESS" | "EGRESS"
	disabled?: bool
	sourceRanges?: [...string]
	allowed?: [...{IPProtocol: string, ports?: [...string], ...}]
	...
}

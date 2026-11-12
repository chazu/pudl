package gcp

// Compute Engine networking as printed by `gcloud compute ... list
// --format=json`. gcloud omits the project, so every collector injects it
// (`set: project`). Identity is the resource's selfLink, which names the
// project, region and resource.
//
// Every type in this pack is a draft: compare it with your own gcloud output,
// then set review: {status: "validated", by: "...", at: "YYYY-MM-DD"}. Until
// then policy results against it are marked provisional.

// The prefix that turns a Compute selfLink into path form.
_gcpComputeAPI: "https://www.googleapis.com/compute/v1/"

// The same, as a fact rewrite: a Compute URL in path form, and no fact when
// the field is absent or not a Compute URL.
_gcpComputePathRewrite: [{match: "^https://www\\.googleapis\\.com/compute/v1/(projects/.+)$", replace: "$1"}]

#Network: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.network"
		identity_fields: ["selfLink"]
		// The relative resource name, the gcp.resource key a Pulumi state
		// resource of this type carries (packs/pulumi).
		same_resource: [{kind: "gcp.resource", key: name: {path: "selfLink", trim_prefix: _gcpComputeAPI}}]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpComputeGraph
		graph: display: {code: "VPC", label: "VPC network", class: "network"}
		graph: network: {group: true, via: ["in_project"]}
		// Network references are joined in path form
		// (projects/P/global/networks/N), see gcp_depends_on_network.
		facts: gcp_network: args: {
			project: "project"
			name:    "name"
			path: {path: "selfLink", trim_prefix: _gcpComputeAPI}
		}
		// One fact per peering: the peer network in path form, and its state
		// (ACTIVE once both sides peer).
		facts: gcp_network_peering: {
			each: "peerings[*]"
			args: {
				network: {path: "network", trim_prefix: _gcpComputeAPI}
				state: {path: "state", default: "UNKNOWN"}
			}
		}
	}
	kind:                   "compute#network" @pudl(match)
	project:                _gcpProjectID
	name:                   string
	selfLink:               string
	autoCreateSubnetworks?: bool
	subnetworks?: [...string]
	...
}

#Subnetwork: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.subnetwork"
		identity_fields: ["selfLink"]
		// The relative resource name, the gcp.resource key a Pulumi state
		// resource of this type carries (packs/pulumi).
		same_resource: [{kind: "gcp.resource", key: name: {path: "selfLink", trim_prefix: _gcpComputeAPI}}]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpComputeGraph & {region: {path: "region", last_segment: true}}
		graph: display: {code: "SUBN", label: "Subnetwork", class: "network"}
		graph: network: {group: true, via: ["in_network"], region: true}
		facts: gcp_subnet: args: {
			project: "project"
			name:    "name"
			// GKE references subnets in path form (projects/P/regions/R/...).
			path: {path: "selfLink", trim_prefix: _gcpComputeAPI}
			network: {path: "network", trim_prefix: _gcpComputeAPI}
			private_google_access: {path: "privateIpGoogleAccess", default: false}
		}
	}
	kind:                   "compute#subnetwork" @pudl(match)
	project:                _gcpProjectID
	name:                   string
	selfLink:               string
	network:                string
	region:                 string
	ipCidrRange:            string
	privateIpGoogleAccess?: bool
	...
}

#Firewall: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.firewall"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpComputeGraph
		graph: display: {code: "FW", label: "Firewall rule", class: "security"}
		graph: network: via: ["in_network"]
		graph: security: control: true
		facts: {
			gcp_firewall_network: args: {
				self_link: "selfLink"
				network: {path: "network", trim_prefix: _gcpComputeAPI}
				// Whom the rule targets: tagged instances, service accounts,
				// or (neither) every instance in the network.
				targeted: {exists: "targetTags"}
				sa_targeted: {exists: "targetServiceAccounts"}
				disabled: {path: "disabled", default: false}
			}
			gcp_firewall_target_tag: args: tag: "targetTags[*]"
			gcp_firewall_target_sa: args: email: "targetServiceAccounts[*]"
			// One fact per source range, with the rule's direction and state.
			gcp_firewall_source: args: {
				range:     "sourceRanges[*]"
				direction: "direction"
				disabled: {path: "disabled", default: false}
			}
			// One fact per allowed protocol and port (port "*" when the rule
			// names no ports). Deny rules have no allowed entries.
			gcp_firewall_allow: {
				each: "allowed[*]"
				args: {protocol: "IPProtocol", port: {path: "ports[*]", default: "*"}}
			}
		}
	}
	kind:      "compute#firewall" @pudl(match)
	project:   _gcpProjectID
	name:      string
	selfLink:  string
	network:   string
	direction: "INGRESS" | "EGRESS"
	disabled?: bool
	sourceRanges?: [...string]
	allowed?: [...{IPProtocol: string, ports?: [...string], ...}]
	denied?: [...{IPProtocol: string, ports?: [...string], ...}]
	...
}

#Route: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.route"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpComputeGraph
		graph: display: {code: "ROUTE", label: "Route", class: "network"}
		graph: network: via: ["in_network"]
		facts: gcp_route_network: args: {
			self_link: "selfLink"
			network: {path: "network", trim_prefix: _gcpComputeAPI}
		}
		// Where a route sends traffic, when that is a resource: an instance,
		// a VPN tunnel, or an internal load balancer's forwarding rule (an
		// ILB given by IP address is not a reference). The internet
		// gateway, peerings and the route's own network are not.
		// Each is a rewrite, so a route without that kind of next hop
		// yields no fact.
		facts: gcp_route_next_hop: args: hop: {path: "nextHopInstance", rewrite: _gcpComputePathRewrite}
		facts: gcp_route_next_hop_vpn: args: hop: {path: "nextHopVpnTunnel", rewrite: _gcpComputePathRewrite}
		facts: gcp_route_next_hop_ilb: args: hop: {path: "nextHopIlb", rewrite: _gcpComputePathRewrite}
	}
	kind:      "compute#route" @pudl(match)
	project:   _gcpProjectID
	name:      string
	selfLink:  string
	network:   string
	destRange: string
	priority?: int
	...
}

#Router: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.router"
		identity_fields: ["selfLink"]
		// The relative resource name, the gcp.resource key a Pulumi state
		// resource of this type carries (packs/pulumi).
		same_resource: [{kind: "gcp.resource", key: name: {path: "selfLink", trim_prefix: _gcpComputeAPI}}]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpComputeGraph & {region: {path: "region", last_segment: true}}
		graph: display: {code: "RTR", label: "Cloud Router", class: "network"}
		graph: network: {via: ["in_network"], region: true}
		facts: {
			gcp_router: args: {
				project:   "project"
				name:      "name"
				self_link: "selfLink"
				network: {path: "network", trim_prefix: _gcpComputeAPI}
			}
			// One fact per NAT IP a Cloud NAT gateway is configured to use.
			gcp_router_nat_ip: {each: "nats[*]", args: {nat: "name", address: "natIps[*]"}}
		}
	}
	kind:     "compute#router" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	network:  string
	region:   string
	nats?: [...{name: string, natIps?: [...string], ...}]
	...
}

#Address: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.address"
		identity_fields: ["selfLink"]
		// The relative resource name, the gcp.resource key a Pulumi state
		// resource of this type carries (packs/pulumi).
		same_resource: [{kind: "gcp.resource", key: name: {path: "selfLink", trim_prefix: _gcpComputeAPI}}]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpComputeGraph & {region: {path: "region", last_segment: true}}
		graph: display: {code: "IP", label: "IP address", class: "network"}
		graph: network: region: true
		facts: gcp_address: args: {
			project:   "project"
			name:      "name"
			self_link: "selfLink"
			status:    "status"
		}
		// What uses the address (an instance, forwarding rule or Cloud NAT
		// router), in path form.
		facts: gcp_address_user: args: user: {path: "users[*]", trim_prefix: _gcpComputeAPI}
	}
	kind:         "compute#address" @pudl(match)
	project:      _gcpProjectID
	name:         string
	selfLink:     string
	address:      string
	status:       "RESERVING" | "RESERVED" | "IN_USE"
	addressType?: "EXTERNAL" | "INTERNAL"
	users?: [...string]
	...
}

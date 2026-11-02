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

#Network: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.network"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		// Network references are joined in path form
		// (projects/P/global/networks/N), see gcp_depends_on_network.
		facts: gcp_network: args: {
			project: "project"
			name:    "name"
			path: {path: "selfLink", trim_prefix: _gcpComputeAPI}
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
		review: status: "draft"
		owner: _#GCPOwner
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
		facts: gcp_firewall_network: args: {
			self_link: "selfLink"
			network: {path: "network", trim_prefix: _gcpComputeAPI}
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
		facts: gcp_route_network: args: {
			self_link: "selfLink"
			network: {path: "network", trim_prefix: _gcpComputeAPI}
		}
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
		review: status: "draft"
		owner: _#GCPOwner
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
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		facts: gcp_address: args: {
			project:   "project"
			name:      "name"
			self_link: "selfLink"
			status:    "status"
		}
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

package gcp

// `gcloud compute instances list --format=json`: every VM in the project,
// all zones. Identity is the selfLink. Instances of the service projects of
// a Shared VPC name the host project's subnetworks, which is how the network
// view places them there.
#ComputeInstance: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.instance"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpComputeGraph & {zone: {path: "zone", last_segment: true}}
		facts: {
			gcp_instance: args: {
				self_link: "selfLink"
				status:    "status"
				zone: {path: "zone", trim_prefix: _gcpComputeAPI}
			}
			// One fact per network interface.
			gcp_instance_nic: {
				each: "networkInterfaces[*]"
				args: {
					network: {path: "network", trim_prefix: _gcpComputeAPI}
					subnetwork: {path: "subnetwork", trim_prefix: _gcpComputeAPI}
					ip: "networkIP"
					external_ip: {exists: "accessConfigs[*].natIP"}
				}
			}
			gcp_instance_sa: args: email: "serviceAccounts[*].email"
			// Network tags: what firewall rules target.
			gcp_instance_tag: args: tag: "tags.items[*]"
		}
	}
	kind:     "compute#instance" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	zone:     string
	status:   string
	machineType?: string
	networkInterfaces?: [...{network?: string, subnetwork?: string, networkIP?: string, ...}]
	serviceAccounts?: [...{email: string, ...}]
	tags?: {items?: [...string], ...}
	...
}

package gcp

// `gcloud compute instances list --format=json`: every VM in the project,
// all zones. Identity is the selfLink. Instances of the service projects of
// a Shared VPC name the host project's subnetworks, which is how the network
// view places them there.
//
// Instance metadata holds startup scripts, user-data and, on GKE nodes,
// kube-env (cluster certificates and bootstrap credentials), so every metadata
// value is redacted before storage. The keys stay: a record still shows which
// metadata items an instance has.
#ComputeInstance: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.instance"
		identity_fields: ["selfLink"]
		sensitive_fields: ["metadata.items[*].value"]
		// Shielded VM Secure Boot state: public UEFI certificates (pk, keks,
		// dbs) and revocation hashes (dbxs), high-entropy but not secret.
		secret_scan_ignore: [
			"disks[*].shieldedInstanceInitialState.pk.content",
			"disks[*].shieldedInstanceInitialState.keks[*].content",
			"disks[*].shieldedInstanceInitialState.dbs[*].content",
			"disks[*].shieldedInstanceInitialState.dbxs[*].content",
		]
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
			// One fact per external IP, with the network it is attached on.
			gcp_instance_external_ip: {
				each: "networkInterfaces[*]"
				args: {
					network: {path: "network", trim_prefix: _gcpComputeAPI}
					address: "accessConfigs[*].natIP"
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
	metadata?: {items?: [...{key: string, value?: _}], ...}
	...
}

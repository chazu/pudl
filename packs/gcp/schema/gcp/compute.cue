package gcp

// `gcloud compute instances list --format=json`: every VM in the project,
// all zones. Identity is the selfLink. Instances of the service projects of
// a Shared VPC name the host project's subnetworks, which is how the network
// view places them there.
//
// Instance metadata holds startup scripts, user-data and, on GKE nodes,
// kube-env (cluster certificates and bootstrap credentials). Every metadata
// value is redacted before storage except those of the keys in
// _gcpSafeMetadataKeys, which hold settings checks need (OS Login, serial
// port, project SSH keys). A key not on the list, including one nobody has
// reviewed yet, stays redacted. The keys themselves are always kept.
#ComputeInstance: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.instance"
		identity_fields: ["selfLink"]
		sensitive_fields: [{
			each:  "metadata.items[*]"
			value: "value"
			keep_when: {field: "key", in: _gcpSafeMetadataKeys}
		}]
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
		graph: display: {code: "GCE", label: "Compute instance", class: "compute"}
		graph: network: via: ["in_subnet"]
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

// Instance metadata keys whose values are settings, not secrets, so
// gcp.#ComputeInstance stores them. To change the list without editing this
// file, set it in another file of package gcp, e.g.
// .pudl/schema/gcp/settings.cue: _gcpSafeMetadataKeys: ["enable-oslogin"]
// Never add a key that can hold a script, a credential or user data
// (startup-script, user-data, ssh-keys, kube-env, gce-container-declaration).
_gcpSafeMetadataKeys: [...string] | *[
	"block-project-ssh-keys",
	"cluster-location",
	"cluster-name",
	"disable-legacy-endpoints",
	"enable-guest-attributes",
	"enable-os-inventory",
	"enable-osconfig",
	"enable-oslogin",
	"enable-oslogin-2fa",
	"google-logging-enabled",
	"google-monitoring-enabled",
	"serial-port-enable",
	"serial-port-logging-enable",
	"vm-dns-setting",
]

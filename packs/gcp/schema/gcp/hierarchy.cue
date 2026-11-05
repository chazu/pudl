package gcp

// The resource hierarchy and what is set on it: organizations, folders,
// organization policies and hierarchical firewall policies. These live above
// projects, so they carry no project (not even an organization policy set on
// a project) and no owner: the project-to-team table cannot name
// them. Collected from Cloud Asset Inventory, searched per organization;
// gcloud's commands walk each organization's folders when CAI cannot answer.
// None of these types holds a secret, so none declares sensitive fields.

// The hierarchy names a node as organizations/N or folders/N.
_gcpHierarchyName: =~"^(organizations|folders)/[0-9]+$"

// gcloud organizations describe / Cloud Asset Inventory
// cloudresourcemanager.googleapis.com/Organization (identical).
#Organization: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.organization"
		identity_fields: ["name"]
		review: status: "draft"
		graph: {name: "displayName", refs: [{path: "name"}]}
		graph: display: {code: "ORG", label: "Organization", class: "admin"}
		facts: gcp_organization: args: {name: "name", display_name: "displayName"}
	}
	name:           =~"^organizations/[0-9]+$" @pudl(match)
	displayName:    string
	lifecycleState: string
	owner?: {directoryCustomerId?: string, ...}
	...
}

// gcloud resource-manager folders list / Cloud Asset Inventory
// cloudresourcemanager.googleapis.com/Folder (identical, Resource Manager v2).
// parent is the folder or organization the folder sits in.
#Folder: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.folder"
		identity_fields: ["name"]
		review: status: "draft"
		graph: {name: "displayName", refs: [{path: "name"}]}
		graph: display: {code: "FLDR", label: "Folder", class: "admin"}
		facts: gcp_folder: args: {name: "name", display_name: "displayName", parent: "parent", state: "lifecycleState"}
	}
	name:           =~"^folders/[0-9]+$" @pudl(match)
	parent:         _gcpHierarchyName
	displayName:    string
	lifecycleState: string
	...
}

// An organization policy (Org Policy API v2): one constraint's policy as set
// on an organization, folder or project, named <resource>/policies/<constraint>.
// The resource is named by number (projects/N for a project). Only policies
// that are set are listed.
#OrgPolicy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.org_policy"
		identity_fields: ["name"]
		review: status: "draft"
		graph: {name: {path: "name", last_segment: true}}
		graph: display: {code: "OPOL", label: "Organization policy", class: "security"}
		facts: gcp_org_policy: args: {
			// The organization, folder or project the policy is set on.
			resource:   {path: "name", rewrite: [{match: "^(.+)/policies/[^/]+$", replace: "$1"}]}
			constraint: {path: "name", rewrite: [{match: "^.*/policies/([^/]+)$", replace: "$1"}]}
			reset:      {path: "spec.reset", default: false}
			inherit:    {path: "spec.inheritFromParent", default: false}
		}
		// One fact per rule. A boolean constraint's rule sets enforce; a
		// list constraint's sets allowAll, denyAll or values.
		facts: gcp_org_policy_rule: {
			each: "spec.rules[*]"
			args: {
				enforce:   {path: "enforce", default: false}
				allow_all: {path: "allowAll", default: false}
				deny_all:  {path: "denyAll", default: false}
				condition: {exists: "condition"}
			}
		}
	}
	name: =~"^(organizations|folders|projects)/[^/]+/policies/[^/]+$" @pudl(match)
	spec?: {
		rules?: [...{...}]
		...
	}
	...
}

// A hierarchical firewall policy (`gcloud compute firewall-policies list
// --organization / --folder`, Cloud Asset Inventory
// compute.googleapis.com/FirewallPolicy): rules set on an organization or
// folder and enforced in every VPC network below the resources it is
// associated with. Global and regional network firewall policies, which
// belong to a project, are not collected.
#FirewallPolicy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.firewall_policy"
		identity_fields: ["selfLink"]
		review: status: "draft"
		graph: {name: "displayName", refs: [{path: "selfLink", trim_prefix: _gcpComputeAPI}]}
		graph: display: {code: "FWP", label: "Hierarchical firewall policy", class: "security"}
		graph: security: control: true
		facts: gcp_firewall_policy: args: parent: "parent"
		// One fact per association: the organization or folder the policy
		// is enforced on (organizations/N or folders/N).
		facts: gcp_firewall_policy_association: {
			each: "associations[*]"
			args: target: "attachmentTarget"
		}
		// One fact per rule and IP protocol. A rule with no protocol list
		// matches every protocol ("all").
		facts: gcp_firewall_policy_rule: {
			each: "rules[*]"
			args: {
				priority:  "priority"
				direction: "direction"
				action:    "action"
				disabled:  {path: "disabled", default: false}
				src_range: {path: "match.srcIpRanges[*]", default: "none"}
			}
		}
	}
	kind:      "compute#firewallPolicy" @pudl(match)
	selfLink:  =~#"^https://www\.googleapis\.com/compute/v1/locations/global/firewallPolicies/[^/]+$"#
	// The policy's numeric ID; displayName and shortName are its name.
	name:         string
	displayName?: string
	shortName?:   string
	parent:       _gcpHierarchyName
	rules?: [...{priority: int, direction: string, action: string, ...}]
	associations?: [...{attachmentTarget: string, ...}]
	...
}

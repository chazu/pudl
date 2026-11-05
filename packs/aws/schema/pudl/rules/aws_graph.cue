package rules

// AWS resource graph edges, read by `pudl graph export` and the web explorer.
// Each rule derives the reserved relation
//   graph_edge(type, source, target, basis)
// from the pack's projected facts (see aws_pack.cue for the relations).
// source is the record's own resource_id; target is the AWS ID or ARN the
// record names, which the exporter resolves to the cataloged resource with
// that identity. A target that was never collected stays a visible
// "not in inventory" reference. Types are the web explorer's relationship
// vocabulary (web/src/model/relationshipTypes.ts). basis is "observed":
// every edge is read from describe-* output, not inferred.

aws_graph_instance_subnet: {
	head: {rel: "graph_edge", args: {type: "in_subnet", source: "$R", target: "$S", basis: "observed"}}
	body: [{rel: "aws_instance", args: {resource_id: "$R", subnet_id: "$S"}}]
}

aws_graph_instance_sg: {
	head: {rel: "graph_edge", args: {type: "protected_by", source: "$R", target: "$G", basis: "observed"}}
	body: [{rel: "aws_instance_sg", args: {resource_id: "$R", group_id: "$G"}}]
}

aws_graph_subnet_vpc: {
	head: {rel: "graph_edge", args: {type: "in_vpc", source: "$R", target: "$V", basis: "observed"}}
	body: [{rel: "aws_subnet", args: {resource_id: "$R", vpc_id: "$V"}}]
}

aws_graph_sg_vpc: {
	head: {rel: "graph_edge", args: {type: "in_vpc", source: "$R", target: "$V", basis: "observed"}}
	body: [{rel: "aws_security_group", args: {resource_id: "$R", vpc_id: "$V"}}]
}

aws_graph_volume_instance: {
	head: {rel: "graph_edge", args: {type: "attached_to", source: "$R", target: "$I", basis: "observed"}}
	body: [{rel: "aws_volume_attachment", args: {resource_id: "$R", instance_id: "$I"}}]
}

aws_graph_volume_key: {
	head: {rel: "graph_edge", args: {type: "encrypted_by", source: "$R", target: "$K", basis: "observed"}}
	body: [{rel: "aws_volume", args: {resource_id: "$R", kms_key: "$K"}}]
}

// An RDS instance names every subnet of its DB subnet group; the explorer
// places it at their common ancestor rather than in one subnet.
aws_graph_db_subnet: {
	head: {rel: "graph_edge", args: {type: "in_subnet", source: "$R", target: "$S", basis: "observed"}}
	body: [{rel: "aws_db_subnet", args: {resource_id: "$R", subnet_id: "$S"}}]
}

aws_graph_db_sg: {
	head: {rel: "graph_edge", args: {type: "protected_by", source: "$R", target: "$G", basis: "observed"}}
	body: [{rel: "aws_db_sg", args: {resource_id: "$R", group_id: "$G"}}]
}

aws_graph_db_key: {
	head: {rel: "graph_edge", args: {type: "encrypted_by", source: "$R", target: "$K", basis: "observed"}}
	body: [{rel: "aws_db_instance", args: {resource_id: "$R", kms_key: "$K"}}]
}

// ---- Security groups: ingress rules ----
// A rule permits traffic; it is not observed traffic (the explorer says so).
// Ranges such as 0.0.0.0/0 become external "address range" nodes.

aws_graph_sg_ingress_group: {
	head: {rel: "graph_edge", args: {type: "allows_ingress_from", source: "$R", target: "$G", basis: "observed"}}
	body: [{rel: "aws_sg_ingress_group", args: {resource_id: "$R", group_id: "$G"}}]
}

aws_graph_sg_ingress_cidr: {
	head: {rel: "graph_edge", args: {type: "allows_ingress_from", source: "$R", target: "$C", basis: "observed"}}
	body: [{rel: "aws_sg_ingress_cidr", args: {resource_id: "$R", cidr: "$C"}}]
}

aws_graph_sg_ingress_cidr6: {
	head: {rel: "graph_edge", args: {type: "allows_ingress_from", source: "$R", target: "$C", basis: "observed"}}
	body: [{rel: "aws_sg_ingress_cidr6", args: {resource_id: "$R", cidr: "$C"}}]
}

// Edge details: protocol and port range of each permitting rule.
aws_graph_sg_group_protocol: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$G", key: "protocol", value: "$P"}}
	body: [{rel: "aws_sg_ingress_group", args: {resource_id: "$R", group_id: "$G", protocol: "$P"}}]
}

aws_graph_sg_group_port: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$G", key: "port", value: "$F"}}
	body: [{rel: "aws_sg_ingress_group", args: {resource_id: "$R", group_id: "$G", from_port: "$F"}}]
}

aws_graph_sg_cidr_protocol: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$C", key: "protocol", value: "$P"}}
	body: [{rel: "aws_sg_ingress_cidr", args: {resource_id: "$R", cidr: "$C", protocol: "$P"}}]
}

aws_graph_sg_cidr_port: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$C", key: "port", value: "$F"}}
	body: [{rel: "aws_sg_ingress_cidr", args: {resource_id: "$R", cidr: "$C", from_port: "$F"}}]
}

aws_graph_sg_cidr6_port: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$C", key: "port", value: "$F"}}
	body: [{rel: "aws_sg_ingress_cidr6", args: {resource_id: "$R", cidr: "$C", from_port: "$F"}}]
}

// ---- IAM: role trust ----
// The trust policy names who may assume the role. Whether that principal's
// own policies allow sts:AssumeRole is not evaluated (the explorer says so).
// A principal that was never collected (another account's root) stays a
// visible reference; "*" becomes the external "Anyone" node.

aws_graph_role_trust: {
	head: {rel: "graph_edge", args: {type: "trust_allows_assume", source: "$P", target: "$R", basis: "observed"}}
	body: [{rel: "aws_role_trust", args: {resource_id: "$R", effect: "Allow", principal: "$P"}}]
}

aws_graph_role_trust_any: {
	head: {rel: "graph_edge", args: {type: "trust_allows_assume", source: "*", target: "$R", basis: "observed"}}
	body: [{rel: "aws_role_trust_any", args: {resource_id: "$R", effect: "Allow", principal: "*"}}]
}

// Only conditional trusts carry the property (a Condition block narrows who
// may assume the role; its terms are not evaluated).
aws_graph_role_trust_condition: {
	head: {rel: "graph_edge_property", args: {type: "trust_allows_assume", source: "$P", target: "$R", key: "conditional", value: "yes"}}
	body: [{rel: "aws_role_trust", args: {resource_id: "$R", effect: "Allow", principal: "$P", conditional: true}}]
}

// ---- Instance profiles ----
// An instance names its instance profile; the profile carries a role.

aws_graph_instance_profile: {
	head: {rel: "graph_edge", args: {type: "has_instance_profile", source: "$R", target: "$P", basis: "observed"}}
	body: [{rel: "aws_instance", args: {resource_id: "$R", instance_profile: "$P"}}]
}

aws_graph_profile_role: {
	head: {rel: "graph_edge", args: {type: "profile_role", source: "$R", target: "$A", basis: "observed"}}
	body: [{rel: "aws_instance_profile_role", args: {resource_id: "$R", role_arn: "$A"}}]
}

// ---- Security groups: egress rules ----

aws_graph_sg_egress_group: {
	head: {rel: "graph_edge", args: {type: "allows_egress_to", source: "$R", target: "$G", basis: "observed"}}
	body: [{rel: "aws_sg_egress_group", args: {resource_id: "$R", group_id: "$G"}}]
}

aws_graph_sg_egress_cidr: {
	head: {rel: "graph_edge", args: {type: "allows_egress_to", source: "$R", target: "$C", basis: "observed"}}
	body: [{rel: "aws_sg_egress_cidr", args: {resource_id: "$R", cidr: "$C"}}]
}

aws_graph_sg_egress_cidr6: {
	head: {rel: "graph_edge", args: {type: "allows_egress_to", source: "$R", target: "$C", basis: "observed"}}
	body: [{rel: "aws_sg_egress_cidr6", args: {resource_id: "$R", cidr: "$C"}}]
}

aws_graph_sg_egress_cidr_protocol: {
	head: {rel: "graph_edge_property", args: {type: "allows_egress_to", source: "$R", target: "$C", key: "protocol", value: "$P"}}
	body: [{rel: "aws_sg_egress_cidr", args: {resource_id: "$R", cidr: "$C", protocol: "$P"}}]
}

aws_graph_sg_egress_cidr_port: {
	head: {rel: "graph_edge_property", args: {type: "allows_egress_to", source: "$R", target: "$C", key: "port", value: "$F"}}
	body: [{rel: "aws_sg_egress_cidr", args: {resource_id: "$R", cidr: "$C", from_port: "$F"}}]
}

// Graph vocabulary for this pack (facts: rules with no body). Edge types
// shared by every provider are pudl built-ins (pudl/rules/graph_vocabulary.cue);
// these are AWS's own. A resource's AccountId names an aws.account, which
// the pack does not collect, so its display info is declared here too.
_awsEdgeTypes: {
	in_vpc:               {category: "containment", label: "in VPC", meaning: "Defined inside this VPC."}
	protected_by:         {category: "security", label: "SG", meaning: "Security group is associated. Association is not a connection."}
	has_instance_profile: {category: "iam", label: "instance profile", meaning: "Instance runs with this instance profile (and the role it carries). Effective permission not evaluated."}
	profile_role:         {category: "iam", label: "profile role", meaning: "Instance profile carries this role; instances launched with it can use the role."}
	trust_allows_assume:  {category: "iam", label: "trust", meaning: "Role trust policy names this principal. The principal’s own permissions are not evaluated."}
}

for id, e in _awsEdgeTypes {
	"graph_edge_type_\(id)": {head: {rel: "graph_edge_type", args: {type: id, category: e.category, label: e.label, meaning: e.meaning}}}
}

graph_provider_aws:              {head: {rel: "graph_provider", args: {provider: "aws", label: "AWS"}}}
graph_account_type_aws:          {head: {rel: "graph_account_type", args: {provider: "aws", type: "aws.account"}}}
graph_resource_type_aws_account: {head: {rel: "graph_resource_type", args: {type: "aws.account", code: "ACCT", label: "AWS account", class: "admin"}}}

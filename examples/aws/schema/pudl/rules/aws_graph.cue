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

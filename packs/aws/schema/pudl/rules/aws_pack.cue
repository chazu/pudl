package rules

// AWS content pack rules. Single-resource best practices are policies
// (schema/aws/policies.cue); these rules threshold policy results, relate
// resources to each other and answer impact queries. Relations come from the
// pack's fact projection and join on AWS IDs and ARNs:
//   aws_vpc(vpc_id, account, region, cidr, is_default)
//   aws_subnet(subnet_id, vpc_id, availability_zone, public_ip_on_launch)
//   aws_security_group(group_id, name, vpc_id)
//   aws_instance(instance_id, subnet_id, vpc_id, state, public_ip)
//   aws_instance_sg(group_id)              one per attached security group
//   aws_volume(volume_id, encrypted, kms_key, state)
//   aws_volume_attachment(instance_id)     one per attachment
//   aws_db_instance(arn, id, subnet_group, vpc_id, encrypted, kms_key)
//   aws_db_subnet(subnet_id), aws_db_sg(group_id)
//   aws_kms_key(arn, key_id, state, manager)
// Every fact also carries entry_id, which joins relations of one record.

// aws_policy_noncompliant_high — a resource whose result for a policy has a
// high or critical violation. Checks select the population with evidence, so
// the rule needs no type filter. The head carries the severity so a policy
// waiver can tell which violations a finding stands for.
aws_policy_noncompliant_high: {
	head: {rel: "aws_policy_noncompliant_high", args: {resource: "$R", policy: "$P", severity: "high"}}
	body: [{rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "high"}}]
}

aws_policy_noncompliant_critical: {
	head: {rel: "aws_policy_noncompliant_high", args: {resource: "$R", policy: "$P", severity: "critical"}}
	body: [{rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "critical"}}]
}

// ---- Relationships ----

// aws_instance_in_vpc — EC2 instance → subnet → VPC, through the subnet's own
// record (not the instance's VpcId), so both must be cataloged.
aws_instance_in_vpc: {
	head: {rel: "aws_instance_in_vpc", args: {instance: "$I", subnet: "$S", vpc: "$V"}}
	body: [
		{rel: "aws_instance", args: {instance_id: "$I", subnet_id: "$S"}},
		{rel: "aws_subnet", args: {subnet_id: "$S", vpc_id: "$V"}},
	]
}

// aws_instance_security_group — EC2 instance → each cataloged security group
// attached to it.
aws_instance_security_group: {
	head: {rel: "aws_instance_security_group", args: {instance: "$I", group: "$G", vpc: "$V"}}
	body: [
		{rel: "aws_instance", args: {entry_id: "$E", instance_id: "$I"}},
		{rel: "aws_instance_sg", args: {entry_id: "$E", group_id: "$G"}},
		{rel: "aws_security_group", args: {group_id: "$G", vpc_id: "$V"}},
	]
}

// aws_db_in_vpc — RDS instance → DB subnet group → VPC (a cataloged VPC).
aws_db_in_vpc: {
	head: {rel: "aws_db_in_vpc", args: {db: "$D", subnet_group: "$G", vpc: "$V"}}
	body: [
		{rel: "aws_db_instance", args: {arn: "$D", subnet_group: "$G", vpc_id: "$V"}},
		{rel: "aws_vpc", args: {vpc_id: "$V"}},
	]
}

// aws_db_in_subnet — RDS instance → each cataloged subnet of its subnet group.
aws_db_in_subnet: {
	head: {rel: "aws_db_in_subnet", args: {db: "$D", subnet: "$S", vpc: "$V"}}
	body: [
		{rel: "aws_db_instance", args: {entry_id: "$E", arn: "$D"}},
		{rel: "aws_db_subnet", args: {entry_id: "$E", subnet_id: "$S"}},
		{rel: "aws_subnet", args: {subnet_id: "$S", vpc_id: "$V"}},
	]
}

// ---- Impact: what depends on this VPC (or subnet, group, instance, key) ----

// aws_uses(resource, kind, uses) — a direct dependency edge: resource, of the
// given kind, needs uses. One rule per edge kind.
aws_uses_subnet_vpc: {
	head: {rel: "aws_uses", args: {resource: "$S", kind: "subnet", uses: "$V"}}
	body: [{rel: "aws_subnet", args: {subnet_id: "$S", vpc_id: "$V"}}]
}

aws_uses_sg_vpc: {
	head: {rel: "aws_uses", args: {resource: "$G", kind: "security_group", uses: "$V"}}
	body: [{rel: "aws_security_group", args: {group_id: "$G", vpc_id: "$V"}}]
}

aws_uses_instance_subnet: {
	head: {rel: "aws_uses", args: {resource: "$I", kind: "instance", uses: "$S"}}
	body: [{rel: "aws_instance", args: {instance_id: "$I", subnet_id: "$S"}}]
}

aws_uses_instance_sg: {
	head: {rel: "aws_uses", args: {resource: "$I", kind: "instance", uses: "$G"}}
	body: [
		{rel: "aws_instance", args: {entry_id: "$E", instance_id: "$I"}},
		{rel: "aws_instance_sg", args: {entry_id: "$E", group_id: "$G"}},
	]
}

aws_uses_volume_instance: {
	head: {rel: "aws_uses", args: {resource: "$X", kind: "volume", uses: "$I"}}
	body: [
		{rel: "aws_volume", args: {entry_id: "$E", volume_id: "$X"}},
		{rel: "aws_volume_attachment", args: {entry_id: "$E", instance_id: "$I"}},
	]
}

aws_uses_volume_key: {
	head: {rel: "aws_uses", args: {resource: "$X", kind: "volume", uses: "$K"}}
	body: [{rel: "aws_volume", args: {volume_id: "$X", kms_key: "$K"}}]
}

// The subnet group names the VPC directly, so a database is found even when
// its subnets are not cataloged.
aws_uses_db_vpc: {
	head: {rel: "aws_uses", args: {resource: "$D", kind: "db_instance", uses: "$V"}}
	body: [{rel: "aws_db_instance", args: {arn: "$D", vpc_id: "$V"}}]
}

aws_uses_db_subnet: {
	head: {rel: "aws_uses", args: {resource: "$D", kind: "db_instance", uses: "$S"}}
	body: [
		{rel: "aws_db_instance", args: {entry_id: "$E", arn: "$D"}},
		{rel: "aws_db_subnet", args: {entry_id: "$E", subnet_id: "$S"}},
	]
}

aws_uses_db_sg: {
	head: {rel: "aws_uses", args: {resource: "$D", kind: "db_instance", uses: "$G"}}
	body: [
		{rel: "aws_db_instance", args: {entry_id: "$E", arn: "$D"}},
		{rel: "aws_db_sg", args: {entry_id: "$E", group_id: "$G"}},
	]
}

aws_uses_db_key: {
	head: {rel: "aws_uses", args: {resource: "$D", kind: "db_instance", uses: "$K"}}
	body: [{rel: "aws_db_instance", args: {arn: "$D", kms_key: "$K"}}]
}

// aws_depends_on(resource, kind, on) — transitive closure of aws_uses:
// resource needs on directly or through other resources. Query it with
// `pudl query aws_depends_on on=<id or ARN>` for the blast radius of any
// resource.
aws_depends_on_base: {
	head: {rel: "aws_depends_on", args: {resource: "$R", kind: "$K", on: "$O"}}
	body: [{rel: "aws_uses", args: {resource: "$R", kind: "$K", uses: "$O"}}]
}

aws_depends_on_rec: {
	head: {rel: "aws_depends_on", args: {resource: "$R", kind: "$K", on: "$O"}}
	body: [
		{rel: "aws_uses", args: {resource: "$R", kind: "$K", uses: "$M"}},
		{rel: "aws_depends_on", args: {resource: "$M", kind: "$MK", on: "$O"}},
	]
}

// aws_depends_on_vpc(vpc, resource, kind) — everything that depends on a
// cataloged VPC: its subnets and security groups, the instances in them, the
// volumes attached to those instances, and databases in its subnets.
//   pudl query aws_depends_on_vpc vpc=vpc-0a1b2c3d4e5f60001
aws_depends_on_vpc: {
	head: {rel: "aws_depends_on_vpc", args: {vpc: "$V", resource: "$R", kind: "$K"}}
	body: [
		{rel: "aws_vpc", args: {vpc_id: "$V"}},
		{rel: "aws_depends_on", args: {resource: "$R", kind: "$K", on: "$V"}},
	]
}

// ---- Cross-resource checks ----

// aws_instance_internet_exposed — a running instance with a public IP address
// attached to a security group that opens a sensitive port (or all traffic)
// to the internet: the group's aws-sg-world-sensitive-port policy violation
// joined to the instance through the attachment.
aws_instance_internet_exposed: {
	head: {rel: "aws_instance_internet_exposed", args: {instance: "$I", group: "$G"}}
	body: [
		{rel: "aws_instance", args: {entry_id: "$E", instance_id: "$I", state: "running", public_ip: true}},
		{rel: "aws_instance_sg", args: {entry_id: "$E", group_id: "$G"}},
		{rel: "aws_security_group", args: {group_id: "$G", resource_id: "$R"}},
		{rel: "policy_violation", args: {resource: "$R", rule: "aws-sg-world-sensitive-port"}},
	]
}

// aws_instance_in_default_vpc — an instance whose subnet belongs to a default
// VPC (created by AWS with public subnets; workloads belong in a designed VPC).
aws_instance_in_default_vpc: {
	head: {rel: "aws_instance_in_default_vpc", args: {instance: "$I", vpc: "$V"}}
	body: [
		{rel: "aws_instance_in_vpc", args: {instance: "$I", vpc: "$V"}},
		{rel: "aws_vpc", args: {vpc_id: "$V", is_default: true}},
	]
}

// aws_db_key_unusable (one rule per key state) — an RDS instance encrypted
// with a KMS key that is disabled or scheduled for deletion: the database
// cannot be restored or started again once the key is gone.
aws_db_key_disabled: {
	head: {rel: "aws_db_key_unusable", args: {db: "$D", key: "$K", state: "Disabled"}}
	body: [
		{rel: "aws_db_instance", args: {arn: "$D", kms_key: "$K"}},
		{rel: "aws_kms_key", args: {arn: "$K", state: "Disabled"}},
	]
}

aws_db_key_pending_deletion: {
	head: {rel: "aws_db_key_unusable", args: {db: "$D", key: "$K", state: "PendingDeletion"}}
	body: [
		{rel: "aws_db_instance", args: {arn: "$D", kms_key: "$K"}},
		{rel: "aws_kms_key", args: {arn: "$K", state: "PendingDeletion"}},
	]
}

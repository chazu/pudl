package models

import (
	"list"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// AWS content pack check families. Each family is checks-only: it reads the
// latest observation of each collector it names (evidence "model:<name>"),
// which must be at most an hour old. Policy results come from the policies in
// schema/aws/policies.cue; the cross-resource checks use rules in
// schema/pudl/rules/aws_pack.cue.

// A check over the latest observations of the named collectors.
_#AWSCheck: {
	_prefix: string
	_from: [...string]
	sm.#Check
	evidence: [for c in _from {"model:\(_prefix)\(c)"}]
	max_age: "1h"
	expect:  "empty"
}

// Fail on any high or critical policy violation; warn on any violation.
_#AWSPolicyChecks: {
	_prefix: string
	_from: [...string]
	_what: string
	let prefix = _prefix
	let from = _from
	out: [
		_#AWSCheck & {
			_prefix:  prefix, _from: from
			name:     "policy-high"
			query:    "aws_policy_noncompliant_high"
			severity: "fail"
			message:  "Resolve high and critical policy violations on \(_what) (pudl policy evaluate --json lists them)"
		},
		_#AWSCheck & {
			_prefix:  prefix, _from: from
			name:     "policy-any"
			query:    "policy_noncompliant"
			severity: "warn"
			message:  "Review every policy violation on \(_what)"
		},
	]
}

// A family: its name, the collectors it runs after, and its checks.
_#AWSFamily: {
	_prefix: string
	_family: string
	_collectors: [...string]
	sm.#SystemModel
	name: "\(_prefix)\(_family)-checks"
	depends_on: [for c in _collectors {"\(_prefix)\(c)"}]
}

_#AWSNetworkExposure: _#AWSFamily & {
	_prefix: string
	let P = _prefix
	_family: "network-exposure"
	_collectors: ["vpcs", "subnets", "security-groups", "instances"]
	checks: list.Concat([(_#AWSPolicyChecks & {_prefix: P, _from: ["security-groups", "instances"], _what: "security groups and EC2 instances"}).out, [
		_#AWSCheck & {
			_prefix: P, _from: ["instances", "security-groups"]
			name:     "internet-exposed-instance"
			query:    "aws_instance_internet_exposed"
			severity: "fail"
			message:  "A running instance with a public IP address is in a security group that opens a sensitive port to the internet"
		},
		_#AWSCheck & {
			_prefix: P, _from: ["instances", "subnets", "vpcs"]
			name:     "instance-in-default-vpc"
			query:    "aws_instance_in_default_vpc"
			severity: "warn"
			message:  "Move instances out of the default VPC into a designed one"
		},
	]])
}

_#AWSDataProtection: _#AWSFamily & {
	_prefix: string
	let P = _prefix
	_family: "data-protection"
	_collectors: ["s3-buckets", "volumes", "db-instances", "kms-keys"]
	checks: list.Concat([(_#AWSPolicyChecks & {_prefix: P, _from: ["s3-buckets", "volumes", "db-instances", "kms-keys"], _what: "S3 buckets, EBS volumes, RDS instances and KMS keys"}).out, [
		_#AWSCheck & {
			_prefix: P, _from: ["db-instances", "kms-keys"]
			name:     "db-key-unusable"
			query:    "aws_db_key_unusable"
			severity: "fail"
			message:  "An RDS instance is encrypted with a KMS key that is disabled or scheduled for deletion"
		},
	]])
}

_#AWSIdentityPosture: _#AWSFamily & {
	_prefix: string
	let P = _prefix
	_family: "identity-posture"
	_collectors: ["iam-users", "iam-roles"]
	checks: (_#AWSPolicyChecks & {_prefix: P, _from: ["iam-users", "iam-roles"], _what: "IAM users and roles"}).out
}

#AWSNetworkExposureChecks: _#AWSNetworkExposure & {_prefix: "aws-"}
#AWSDataProtectionChecks: _#AWSDataProtection & {_prefix: "aws-"}
#AWSIdentityPostureChecks: _#AWSIdentityPosture & {_prefix: "aws-"}
#AWSFixtureNetworkExposureChecks: _#AWSNetworkExposure & {_prefix: "aws-fixture-"}
#AWSFixtureDataProtectionChecks: _#AWSDataProtection & {_prefix: "aws-fixture-"}
#AWSFixtureIdentityPostureChecks: _#AWSIdentityPosture & {_prefix: "aws-fixture-"}

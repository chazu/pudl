package aws

import "list"

// Single-resource best practices, evaluated against every resource of the
// named type (docs/design/2026-11-01-compliance-policies.md). Policies are
// standalone definitions linked to their type by base_schema.
//
// Absence: a constrained field the record lacks makes the result unknown,
// unless the field carries absent="violation". That is used only where the
// collector leaves a field out to mean the setting is off.
//
// Conditions: a field is annotated outside the comprehension that constrains
// it. CUE's && and || do not short-circuit over a field the record lacks, so a
// condition that must treat an absent field as false tests the list of
// present values ([if f != _|_ {f}]) with list.Contains. A rule that depends
// on several fields is written as a conflict on a field every record carries
// (the anchor), because a conditional requirement cannot detect an absent
// field.

// Ports AWS Security Hub (EC2.19) treats as high risk when open to the
// internet.
_awsSensitivePorts: [20, 21, 22, 23, 25, 110, 135, 143, 445, 1433, 1434, 3000, 3306, 3389, 4333, 5000, 5432, 5500, 5601, 8080, 8088, 8888, 9200, 9300]

// Security groups: no ingress permission open to 0.0.0.0/0 or ::/0 that covers
// a sensitive TCP or UDP port, or allows all traffic (IpProtocol "-1"). A
// permission's FromPort..ToPort range covers a port when it includes it.
// Ingress from other security groups or prefix lists is not flagged.
#SecurityGroupBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#SecurityGroup"
		description: "No sensitive port or all traffic open to the internet"
	}
	IpPermissions?: [...{
		IpProtocol?: _
		FromPort?:   _
		ToPort?:     _
		IpRanges?: [...{CidrIp?: _ @pudl(rule="aws-sg-world-sensitive-port", severity="high")}]
		Ipv6Ranges?: [...{CidrIpv6?: _ @pudl(rule="aws-sg-world-sensitive-port", severity="high")}]
		_protocol: [if IpProtocol != _|_ {IpProtocol}]
		_ported: [for p in _protocol if list.Contains(["tcp", "udp", "6", "17"], p) {p}]
		_covered: [if len(_ported) > 0 if FromPort != _|_ if ToPort != _|_ for port in _awsSensitivePorts if FromPort <= port if ToPort >= port {port}]
		if list.Contains(_protocol, "-1") || len(_covered) > 0 {
			IpRanges?: [...{CidrIp?: !="0.0.0.0/0"}]
			Ipv6Ranges?: [...{CidrIpv6?: !="::/0"}]
		}
	}]
}

// EC2 instances: the instance metadata service requires session tokens
// (IMDSv2), unless the metadata endpoint is disabled.
#EC2InstanceBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#EC2Instance"
		description: "IMDSv2 required"
	}
	MetadataOptions?: {
		HttpEndpoint?: _
		HttpTokens?:   _ @pudl(rule="aws-ec2-imdsv2", severity="medium")
		if !list.Contains([if HttpEndpoint != _|_ {HttpEndpoint}], "disabled") {
			HttpTokens?: "required"
		}
	}
}

// EBS volumes are encrypted. describe-volumes always prints Encrypted.
#EBSVolumeBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#EBSVolume"
		description: "Volumes are encrypted"
	}
	Encrypted: true @pudl(rule="aws-ebs-encryption", severity="high")
}

// S3 buckets. The collector leaves PublicAccessBlockConfiguration out when the
// bucket has none and Versioning out when versioning was never enabled; both
// are violations. The account-level public access block is not collected, so
// a bucket covered only by it is still reported.
#S3BucketBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#S3Bucket"
		description: "Bucket-level public access block on, versioning enabled"
	}
	PublicAccessBlockConfiguration: {
		BlockPublicAcls:       true @pudl(rule="aws-s3-block-public-access", severity="high")
		IgnorePublicAcls:      true @pudl(rule="aws-s3-block-public-access", severity="high")
		BlockPublicPolicy:     true @pudl(rule="aws-s3-block-public-access", severity="high")
		RestrictPublicBuckets: true @pudl(rule="aws-s3-block-public-access", severity="high")
	} @pudl(absent="violation")
	Versioning: {
		Status: "Enabled" @pudl(rule="aws-s3-versioning", severity="medium")
	} @pudl(absent="violation")
}

// RDS instances: encrypted storage, not publicly accessible, automated
// backups (a retention period of 0 disables them). describe-db-instances
// always prints all three fields.
#RDSInstanceBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#RDSInstance"
		description: "Encrypted, private, backed up"
	}
	StorageEncrypted:      true  @pudl(rule="aws-rds-storage-encryption", severity="high")
	PubliclyAccessible:    false @pudl(rule="aws-rds-public-access", severity="high")
	BackupRetentionPeriod: >0    @pudl(rule="aws-rds-backups", severity="medium")
}

// KMS keys. Only enabled, customer managed, symmetric keys can rotate
// automatically on request (AWS managed keys always rotate). When such a key
// does not rotate, the violation is reported on KeySpec, the field that
// decides whether rotation is possible. A missing KeyRotationEnabled counts
// as not rotating.
#KMSKeyBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#KMSKey"
		description: "Customer managed symmetric keys rotate automatically"
	}
	KeyRotationEnabled?: _
	KeyMetadata: {
		KeyManager?: _
		KeyState?:   _
		KeySpec?:    _ @pudl(rule="aws-kms-rotation", severity="medium")
	}
	_rotates: list.Contains([if KeyRotationEnabled != _|_ {KeyRotationEnabled}], true)
	_customer: list.Contains([if KeyMetadata.KeyManager != _|_ {KeyMetadata.KeyManager}], "CUSTOMER")
	_enabled: list.Contains([if KeyMetadata.KeyState != _|_ {KeyMetadata.KeyState}], "Enabled")
	if _customer && _enabled && !_rotates {
		KeyMetadata: KeySpec?: !="SYMMETRIC_DEFAULT"
	}
}

// IAM users with a console password (LoginProfile) have an MFA device. The
// collector leaves LoginProfile out for users without console access.
#IAMUserBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#IAMUser"
		description: "Console users have MFA"
	}
	LoginProfile?: _
	MFADevices?:   _ @pudl(rule="aws-iam-console-mfa", severity="high")
	if LoginProfile != _|_ {
		MFADevices?: [_, ...]
	}
}

// IAM roles: the trust policy does not let any principal ("*", or AWS "*")
// assume the role through an Allow statement without a Condition. Statement
// may be one object or a list. The violation is reported on RoleName, the
// anchor every role has.
#IAMRoleBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#IAMRole"
		description: "Trust policy does not admit any principal unconditionally"
	}
	RoleName?:                 _ @pudl(rule="aws-iam-role-public-trust", severity="critical")
	AssumeRolePolicyDocument?: _
	_documents: [if AssumeRolePolicyDocument != _|_ {AssumeRolePolicyDocument}]
	_statements: [for d in _documents if d.Statement != _|_ for s in list.FlattenN([d.Statement], 1) {s}]
	_unconditional: [for s in _statements
		if list.Contains([if s.Effect != _|_ {s.Effect}], "Allow")
		if s.Condition == _|_ {s}]
	_anyone: [for s in _unconditional if (s.Principal & "*") != _|_ {s}]
	_anyAWS: [for s in _unconditional if s.Principal.AWS != _|_
		for a in list.FlattenN([s.Principal.AWS], 1) if a == "*" {s}]
	_public: list.Concat([_anyone, _anyAWS])
	if len(_public) > 0 {
		RoleName?: "the trust policy lets any principal assume this role without a condition"
	}
}

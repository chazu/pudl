package models

import (
	"list"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// AWS content pack models (README: examples/aws/README.md in the PUDL source).
//
// One collector per resource type, in two variants:
//   aws-<type>          runs the AWS CLI against every profile in your config;
//   aws-fixture-<type>  reads the bundled fixtures in .pudl/populators/aws.
// Three checks-only families evaluate the latest collector observations:
//   aws[-fixture]-network-exposure-checks, -data-protection-checks and
//   -identity-posture-checks.
// Run a family together with its collectors, which it depends on, e.g.
//   pudl check aws-vpcs aws-subnets aws-security-groups aws-instances aws-network-exposure-checks

// ---- Live settings. Each has a default; to change one without editing this
// file, set it in another file of package models, e.g.
// .pudl/schema/models/aws_settings.cue: _awsProfileFilter: "*-readonly"

// The live collectors fan out over the AWS CLI profiles in your config
// (`aws configure list-profiles`), discovered at run time; each run reads its
// profile's account ID with `aws sts get-caller-identity`. A shell glob
// narrows the profiles, e.g. "*-readonly"; "*" takes them all.
_awsProfileFilter: string | *"*"

// Regional collectors list every region enabled in each profile's account
// (`aws ec2 describe-regions`) whose name matches this filter, e.g. "us-*" or
// "eu-west-1"; "*" takes them all. S3 and IAM are global.
_awsRegionFilter: string | *"*"

// ---- End of live settings. ----

// The schema each collector routes its records to.
_awsSchemas: {
	vpcs:              "aws.#VPC"
	subnets:           "aws.#Subnet"
	"security-groups": "aws.#SecurityGroup"
	instances:         "aws.#EC2Instance"
	volumes:           "aws.#EBSVolume"
	"db-instances":    "aws.#RDSInstance"
	"kms-keys":        "aws.#KMSKey"
	"s3-buckets":      "aws.#S3Bucket"
	"iam-users":       "aws.#IAMUser"
	"iam-roles":       "aws.#IAMRole"
	"iam-instance-profiles": "aws.#IAMInstanceProfile"
}

// Regional list commands, and the JMESPath query that unwraps the response's
// top-level list into a JSON array of records; the CLI follows pagination.
_awsRegional: {
	vpcs: {query: "Vpcs", args: ["ec2", "describe-vpcs"]}
	subnets: {query: "Subnets", args: ["ec2", "describe-subnets"]}
	"security-groups": {query: "SecurityGroups", args: ["ec2", "describe-security-groups"]}
	instances: {query: "Reservations[].Instances[]", args: ["ec2", "describe-instances"]}
	volumes: {query: "Volumes", args: ["ec2", "describe-volumes"]}
	"db-instances": {query: "DBInstances", args: ["rds", "describe-db-instances"]}
}

// Every live collector is a short POSIX shell script around the AWS CLI;
// settings only available per resource are read per resource. opt KEY CODES ARGS... prints ,"KEY":<JSON>
// with the output of `aws ARGS`, or nothing when aws fails with one of the
// |-separated error CODES, which mean "not configured". Any other error fails
// the run (a failed input). account PROFILE prints the profile's account ID;
// regions PROFILE FILTER prints the enabled regions matching FILTER, asked of
// the profile's configured region (us-east-1 when it has none). AWS omits the
// account and region from most records, so every script adds AccountId (and
// Region) to each record it prints. Scripts take the profile as $1 and the
// region filter, where regions apply, as $2.
_awsShPrelude: """
	set -eu
	opt() {
		key=$1 codes=$2
		shift 2
		if out=$(aws "$@" --output json 2>&1); then
			printf ',"%s":%s' "$key" "$out"
			return 0
		fi
		for code in $(printf '%s' "$codes" | tr '|' ' '); do
			case $out in *"$code"*) return 0 ;; esac
		done
		printf '%s\\n' "$out" >&2
		return 1
	}
	account() {
		aws sts get-caller-identity --profile "$1" --query Account --output text
	}
	regions() {
		home=$(aws configure get region --profile "$1" || echo us-east-1)
		aws ec2 describe-regions --profile "$1" --region "$home" --filters "Name=region-name,Values=$2" --query 'Regions[].RegionName' --output text
	}
	"""

_awsScripts: {
	// Any regional list command: $3 is the JMESPath query that unwraps its
	// records, and the rest is the command. Each record gains AccountId and
	// Region through JMESPath merge().
	regional: """
		\(_awsShPrelude)
		p=$1 rf=$2 query=$3
		shift 3
		a=$(account "$p")
		rs=$(regions "$p" "$rf")
		for r in $rs; do
			fields=$(printf '`{"AccountId":"%s","Region":"%s"}`' "$a" "$r")
			aws "$@" --profile "$p" --region "$r" --output json --query "$query | [].merge(@, $fields)"
		done
		"""
	// One record per bucket: list-buckets, then the public access block,
	// default encryption and versioning status of each bucket.
	"s3-buckets": """
		\(_awsShPrelude)
		p=$1
		a=$(account "$p")
		buckets=$(aws s3api list-buckets --profile "$p" --query 'Buckets[].[Name,CreationDate]' --output text)
		printf '%s\\n' "$buckets" | while read -r b created; do
			[ -n "$b" ] || continue
			pab=$(opt PublicAccessBlockConfiguration NoSuchPublicAccessBlockConfiguration s3api get-public-access-block --profile "$p" --bucket "$b" --query PublicAccessBlockConfiguration)
			enc=$(opt ServerSideEncryptionConfiguration ServerSideEncryptionConfigurationNotFoundError s3api get-bucket-encryption --profile "$p" --bucket "$b" --query ServerSideEncryptionConfiguration)
			status=$(aws s3api get-bucket-versioning --profile "$p" --bucket "$b" --query Status --output json)
			ver=
			[ "$status" = null ] || ver=$(printf ',"Versioning":{"Status":%s}' "$status")
			printf '{"Name":"%s","CreationDate":"%s","AccountId":"%s"%s%s%s}\\n' "$b" "$created" "$a" "$pab" "$enc" "$ver"
		done
		"""
	// One record per key: describe-key, and rotation status where AWS can
	// report it.
	"kms-keys": """
		\(_awsShPrelude)
		p=$1 rf=$2
		a=$(account "$p")
		rs=$(regions "$p" "$rf")
		for r in $rs; do
			keys=$(aws kms list-keys --profile "$p" --region "$r" --query 'Keys[].KeyId' --output text)
			for k in $keys; do
				meta=$(aws kms describe-key --profile "$p" --region "$r" --key-id "$k" --query KeyMetadata --output json)
				rot=$(opt KeyRotationEnabled 'UnsupportedOperationException|KMSInvalidStateException' kms get-key-rotation-status --profile "$p" --region "$r" --key-id "$k" --query KeyRotationEnabled)
				printf '{"KeyMetadata":%s,"AccountId":"%s","Region":"%s"%s}\\n' "$meta" "$a" "$r" "$rot"
			done
		done
		"""
	// One record per user: get-user, the login profile if the user has a
	// console password, and MFA devices.
	"iam-users": """
		\(_awsShPrelude)
		p=$1
		a=$(account "$p")
		users=$(aws iam list-users --profile "$p" --query 'Users[].UserName' --output text)
		for u in $users; do
			user=$(aws iam get-user --profile "$p" --user-name "$u" --query User --output json)
			login=$(opt LoginProfile NoSuchEntity iam get-login-profile --profile "$p" --user-name "$u" --query LoginProfile)
			mfa=$(aws iam list-mfa-devices --profile "$p" --user-name "$u" --query MFADevices --output json)
			printf '{"User":%s,"MFADevices":%s,"AccountId":"%s"%s}\\n' "$user" "$mfa" "$a" "$login"
		done
		"""
	"iam-roles": """
		\(_awsShPrelude)
		p=$1
		a=$(account "$p")
		fields=$(printf '`{"AccountId":"%s"}`' "$a")
		aws iam list-roles --profile "$p" --output json --query "Roles | [].merge(@, $fields)"
		"""
	"iam-instance-profiles": """
		\(_awsShPrelude)
		p=$1
		a=$(account "$p")
		fields=$(printf '`{"AccountId":"%s"}`' "$a")
		aws iam list-instance-profiles --profile "$p" --output json --query "InstanceProfiles | [].merge(@, $fields)"
		"""
}

// The enumeration every live collector repeats its runs over: one input per
// AWS CLI profile matching _awsProfileFilter.
_awsProfileList: argv: ["sh", "-c", """
	set -eu
	profiles=$(aws configure list-profiles)
	for p in $profiles; do
		case $p in $1) printf '"%s"\\n' "$p" ;; esac
	done
	""", "aws-profiles", _awsProfileFilter]

// Live runs, one per enumerated profile ({{item}}). Regional collectors walk
// the profile's enabled regions inside the run, so a failing region fails
// that profile's input.
_awsRuns: {
	for t, c in _awsRegional {
		(t): [{argv: list.Concat([["sh", "-c", _awsScripts.regional, "aws-\(t)", "{{item}}", _awsRegionFilter, c.query], c.args])}]
	}
	"kms-keys": [{argv: ["sh", "-c", _awsScripts["kms-keys"], "aws-kms-keys", "{{item}}", _awsRegionFilter]}]
	for t in ["s3-buckets", "iam-users", "iam-roles", "iam-instance-profiles"] {
		(t): [{argv: ["sh", "-c", _awsScripts[t], "aws-\(t)", "{{item}}"]}]
	}
}

// Global types carry no Region.
_awsGlobal: {"s3-buckets": true, "iam-users": true, "iam-roles": true, "iam-instance-profiles": true}

// A collector observes one type completely. A run that fails is a failed
// input: the other runs are ingested and the observation is incomplete.
_#AWSCollector: {
	_type:   string
	_prefix: string
	_scope:  string
	_runs: [...]
	sm.#SystemModel
	name: "\(_prefix)\(_type)"
	observation: {scope: "\(_scope)/\(_type)", complete: true}
	populate: sm.#CommandObserve & {schema: _awsSchemas[_type], runs: _runs}
}

_#AWSLive: _#AWSCollector & {
	_type:   string
	_prefix: "aws-"
	_scope:  "aws"
	_runs:   _awsRuns[_type]
	populate: each: _awsProfileList
}

// Fixtures hold what the live command prints, for account 123456789012 in
// us-east-1.
_#AWSFixture: _#AWSCollector & {
	_type:   string
	_prefix: "aws-fixture-"
	_scope:  "fixture/aws"
	_runs: [{
		argv: ["cat", "../../populators/aws/\(_type).json"]
		if _awsGlobal[_type] != _|_ {set: AccountId: "123456789012"}
		if _awsGlobal[_type] == _|_ {set: {AccountId: "123456789012", Region: "us-east-1"}}
	}]
}

#AWSVPCs: _#AWSLive & {_type: "vpcs"}
#AWSSubnets: _#AWSLive & {_type: "subnets"}
#AWSSecurityGroups: _#AWSLive & {_type: "security-groups"}
#AWSInstances: _#AWSLive & {_type: "instances"}
#AWSVolumes: _#AWSLive & {_type: "volumes"}
#AWSDBInstances: _#AWSLive & {_type: "db-instances"}
#AWSKMSKeys: _#AWSLive & {_type: "kms-keys"}
#AWSS3Buckets: _#AWSLive & {_type: "s3-buckets"}
#AWSIAMUsers: _#AWSLive & {_type: "iam-users"}
#AWSIAMRoles: _#AWSLive & {_type: "iam-roles"}
#AWSIAMInstanceProfiles: _#AWSLive & {_type: "iam-instance-profiles"}
#AWSFixtureVPCs: _#AWSFixture & {_type: "vpcs"}
#AWSFixtureSubnets: _#AWSFixture & {_type: "subnets"}
#AWSFixtureSecurityGroups: _#AWSFixture & {_type: "security-groups"}
#AWSFixtureInstances: _#AWSFixture & {_type: "instances"}
#AWSFixtureVolumes: _#AWSFixture & {_type: "volumes"}
#AWSFixtureDBInstances: _#AWSFixture & {_type: "db-instances"}
#AWSFixtureKMSKeys: _#AWSFixture & {_type: "kms-keys"}
#AWSFixtureS3Buckets: _#AWSFixture & {_type: "s3-buckets"}
#AWSFixtureIAMUsers: _#AWSFixture & {_type: "iam-users"}
#AWSFixtureIAMRoles: _#AWSFixture & {_type: "iam-roles"}
#AWSFixtureIAMInstanceProfiles: _#AWSFixture & {_type: "iam-instance-profiles"}

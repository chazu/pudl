package models

import (
	"list"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// AWS content pack models (README: examples/aws/README.md in the PUDL source).
//
// One collector per resource type, in two variants:
//   aws-<type>          runs the AWS CLI against the accounts and regions below;
//   aws-fixture-<type>  reads the bundled fixtures in .pudl/populators/aws.
// Three checks-only families evaluate the latest collector observations:
//   aws[-fixture]-network-exposure-checks, -data-protection-checks and
//   -identity-posture-checks.
// Run a family together with its collectors, which it depends on, e.g.
//   pudl check aws-vpcs aws-subnets aws-security-groups aws-instances aws-network-exposure-checks

// ---- Live settings: replace these before running the aws-* models. ----

// Account IDs, each mapped to the AWS CLI profile (~/.aws/config) that reads
// it. Every live collector runs once per account.
_awsAccounts: {
	"REPLACE_WITH_ACCOUNT_ID": "REPLACE_WITH_PROFILE"
}

// Regions every regional collector lists in each account. S3 and IAM are
// global and run once per account.
_awsRegions: ["us-east-1"]

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
}

// Regional list commands. --query unwraps the response's top-level list so
// the CLI prints a JSON array of records; the CLI follows pagination.
_awsRegional: {
	vpcs: ["ec2", "describe-vpcs", "--query", "Vpcs"]
	subnets: ["ec2", "describe-subnets", "--query", "Subnets"]
	"security-groups": ["ec2", "describe-security-groups", "--query", "SecurityGroups"]
	instances: ["ec2", "describe-instances", "--query", "Reservations[].Instances[]"]
	volumes: ["ec2", "describe-volumes", "--query", "Volumes"]
	"db-instances": ["rds", "describe-db-instances", "--query", "DBInstances"]
}

// Settings that are only available per resource are read by a short POSIX
// shell script around the AWS CLI. opt KEY CODES ARGS... prints ,"KEY":<JSON>
// with the output of `aws ARGS`, or nothing when aws fails with one of the
// |-separated error CODES, which mean "not configured". Any other error fails
// the run (a failed input). Scripts take the profile as $1 and the
// region, where one applies, as $2, and print one JSON object per resource.
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
	"""

_awsScripts: {
	// One record per bucket: list-buckets, then the public access block,
	// default encryption and versioning status of each bucket.
	"s3-buckets": """
		\(_awsShPrelude)
		p=$1
		buckets=$(aws s3api list-buckets --profile "$p" --query 'Buckets[].[Name,CreationDate]' --output text)
		printf '%s\\n' "$buckets" | while read -r b created; do
			[ -n "$b" ] || continue
			pab=$(opt PublicAccessBlockConfiguration NoSuchPublicAccessBlockConfiguration s3api get-public-access-block --profile "$p" --bucket "$b" --query PublicAccessBlockConfiguration)
			enc=$(opt ServerSideEncryptionConfiguration ServerSideEncryptionConfigurationNotFoundError s3api get-bucket-encryption --profile "$p" --bucket "$b" --query ServerSideEncryptionConfiguration)
			status=$(aws s3api get-bucket-versioning --profile "$p" --bucket "$b" --query Status --output json)
			ver=
			[ "$status" = null ] || ver=$(printf ',"Versioning":{"Status":%s}' "$status")
			printf '{"Name":"%s","CreationDate":"%s"%s%s%s}\\n' "$b" "$created" "$pab" "$enc" "$ver"
		done
		"""
	// One record per key: describe-key, and rotation status where AWS can
	// report it.
	"kms-keys": """
		\(_awsShPrelude)
		p=$1 r=$2
		keys=$(aws kms list-keys --profile "$p" --region "$r" --query 'Keys[].KeyId' --output text)
		for k in $keys; do
			meta=$(aws kms describe-key --profile "$p" --region "$r" --key-id "$k" --query KeyMetadata --output json)
			rot=$(opt KeyRotationEnabled 'UnsupportedOperationException|KMSInvalidStateException' kms get-key-rotation-status --profile "$p" --region "$r" --key-id "$k" --query KeyRotationEnabled)
			printf '{"KeyMetadata":%s%s}\\n' "$meta" "$rot"
		done
		"""
	// One record per user: get-user, the login profile if the user has a
	// console password, and MFA devices.
	"iam-users": """
		\(_awsShPrelude)
		p=$1
		users=$(aws iam list-users --profile "$p" --query 'Users[].UserName' --output text)
		for u in $users; do
			user=$(aws iam get-user --profile "$p" --user-name "$u" --query User --output json)
			login=$(opt LoginProfile NoSuchEntity iam get-login-profile --profile "$p" --user-name "$u" --query LoginProfile)
			mfa=$(aws iam list-mfa-devices --profile "$p" --user-name "$u" --query MFADevices --output json)
			printf '{"User":%s,"MFADevices":%s%s}\\n' "$user" "$mfa" "$login"
		done
		"""
}

// Live runs: accounts x regions for regional types, accounts for global ones.
// AWS omits the account and region from most records, so every run sets them.
_awsRuns: {
	for t, args in _awsRegional {
		(t): [for a, p in _awsAccounts for r in _awsRegions {
			argv: list.Concat([["aws"], args, ["--profile", p, "--region", r, "--output", "json"]])
			set: {AccountId: a, Region: r}
		}]
	}
	"kms-keys": [for a, p in _awsAccounts for r in _awsRegions {
		argv: ["sh", "-c", _awsScripts["kms-keys"], "aws-kms-keys", p, r]
		set: {AccountId: a, Region: r}
	}]
	"s3-buckets": [for a, p in _awsAccounts {
		argv: ["sh", "-c", _awsScripts["s3-buckets"], "aws-s3-buckets", p]
		set: AccountId: a
	}]
	"iam-users": [for a, p in _awsAccounts {
		argv: ["sh", "-c", _awsScripts["iam-users"], "aws-iam-users", p]
		set: AccountId: a
	}]
	"iam-roles": [for a, p in _awsAccounts {
		argv: ["aws", "iam", "list-roles", "--query", "Roles", "--profile", p, "--output", "json"]
		set: AccountId: a
	}]
}

// Global types carry no Region.
_awsGlobal: {"s3-buckets": true, "iam-users": true, "iam-roles": true}

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

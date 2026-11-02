# AWS content pack

An installable set of CUE files for cataloging AWS with PUDL: draft schemas
for the core inventory, `aws` CLI collectors, single-resource best-practice
policies, relationship and impact rules, cross-resource checks and offline
fixtures. It contains no vendor-specific Go.

```bash
pudl init
pudl example install aws
```

Installed files, under `.pudl/`:

| Path | Contents |
| --- | --- |
| `schema/aws/{network,compute,data,iam}.cue` | Type schemas (package `aws`), all `review: status: "draft"` |
| `schema/aws/owners.cue` | Ownership: the account-to-team table the types share |
| `schema/aws/policies.cue` | Best-practice policies |
| `schema/pudl/rules/aws_pack.cue` | Datalog rules: relationships, impact, checks |
| `schema/models/aws_pack.cue` | Collectors, and the optional live settings |
| `schema/models/aws_pack_checks.cue` | The three check families |
| `populators/aws/*.json` | Fixtures: CLI-shaped output for account `123456789012` in `us-east-1` |

## Relation to the built-in `pudl/aws` schemas

Every workspace already has `pudl/aws.#VPC`, `#Subnet`, `#SecurityGroup` and
other network types from `pudl init`. The pack's `aws.#VPC`, `aws.#Subnet` and
`aws.#SecurityGroup` specialize them (`base_schema`) with the same identity
fields (`VpcId`, `SubnetId`, `GroupId`), so one VPC has one resource ID
whichever schema classified it, a policy on the built-in type also applies to
the pack's, and a plain `pudl import` of `describe-vpcs` output that lacks the
collector's `AccountId` and `Region` still falls back to the built-in type.
The pack adds what the built-ins lack: review status, ownership and fact
projection. The built-in `pudl/aws.#Instance` is a normalized snake_case shape
rather than CLI output, so the pack's `aws.#EC2Instance` is a separate type;
the other pack types (volumes, S3, RDS, KMS, IAM) have no built-in
counterpart.

## What it collects

Each collector is a `#CommandObserve` model. A live collector lists your AWS
CLI profiles at run time (`aws configure list-profiles`) and runs once per
profile: a short POSIX `sh` script around the CLI, held in the model file,
reads the profile's account ID (`aws sts get-caller-identity`), walks its
enabled regions for regional types (`aws ec2 describe-regions`), adds
`AccountId` (and `Region`) to every record and routes the records to one
schema. List commands use `--query` to unwrap the response's top-level list
(`{"Vpcs": [...]}` becomes `[...]`) and JMESPath `merge()` to add the
fields; the CLI follows pagination. Settings that AWS only reports per
resource (S3 bucket settings, KMS key metadata and rotation, IAM login
profiles and MFA) are read per resource, one JSON object each. A setting that is not configured
(for example `NoSuchPublicAccessBlockConfiguration`) is left out of the
record; any other error fails the run.

Each collector declares a complete observation of scope `aws/<type>`
(`fixture/aws/<type>` for fixtures). If one profile's run fails (a missing
permission, an expired session, a failing region), the other profiles are
ingested, the observation is incomplete, and the collector fails with that
profile listed under `populate.failed_inputs` in its report. A family checked with it is blocked;
checked later on its own, its checks over that observation are unknown, not
passing.

| Collector | Command | Schema | Identity | IAM actions |
| --- | --- | --- | --- | --- |
| `aws-vpcs` | `ec2 describe-vpcs` | `aws.#VPC` | `VpcId` | `ec2:DescribeVpcs` |
| `aws-subnets` | `ec2 describe-subnets` | `aws.#Subnet` | `SubnetId` | `ec2:DescribeSubnets` |
| `aws-security-groups` | `ec2 describe-security-groups` | `aws.#SecurityGroup` | `GroupId` | `ec2:DescribeSecurityGroups` |
| `aws-instances` | `ec2 describe-instances` | `aws.#EC2Instance` | `InstanceId` | `ec2:DescribeInstances` |
| `aws-volumes` | `ec2 describe-volumes` | `aws.#EBSVolume` | `VolumeId` | `ec2:DescribeVolumes` |
| `aws-db-instances` | `rds describe-db-instances` | `aws.#RDSInstance` | `DBInstanceArn` | `rds:DescribeDBInstances` |
| `aws-kms-keys` | `kms list-keys`, then per key `describe-key`, `get-key-rotation-status` | `aws.#KMSKey` | `KeyMetadata.Arn` | `kms:ListKeys`, `kms:DescribeKey`, `kms:GetKeyRotationStatus` |
| `aws-s3-buckets` | `s3api list-buckets`, then per bucket `get-public-access-block`, `get-bucket-encryption`, `get-bucket-versioning` | `aws.#S3Bucket` | `Name` | `s3:ListAllMyBuckets`, `s3:GetBucketPublicAccessBlock`, `s3:GetEncryptionConfiguration`, `s3:GetBucketVersioning` |
| `aws-iam-users` | `iam list-users`, then per user `get-user`, `get-login-profile`, `list-mfa-devices` | `aws.#IAMUser` | `User.Arn` | `iam:ListUsers`, `iam:GetUser`, `iam:GetLoginProfile`, `iam:ListMFADevices` |
| `aws-iam-roles` | `iam list-roles` | `aws.#IAMRole` | `Arn` | `iam:ListRoles` |
| `aws-iam-instance-profiles` | `iam list-instance-profiles` | `aws.#IAMInstanceProfile` | `Arn` | `iam:ListInstanceProfiles` |

Every live collector also calls `sts:GetCallerIdentity`, and regional ones
`ec2:DescribeRegions`. The AWS managed `SecurityAudit` (or `ReadOnlyAccess`)
policy covers every collector. S3 and IAM are global: their records carry no
`Region`. The scripts need `sh`, `tr` and the AWS CLI
v2; nothing else.

EC2 user data (`UserData`) is redacted before anything is stored.
`describe-instances` does not print it, but records enriched with
`describe-instance-attribute --attribute userData` would carry bootstrap
scripts and secrets. No collector reads secrets: login profiles only show that
a console password exists, and access keys are not collected.

The schemas are drafts. Compare each with your own CLI output, then set
`review: {status: "validated", by: "...", at: "YYYY-MM-DD"}`. Until then
policy results are marked provisional.

## Ownership

Every type declares `_pudl: owner`: a resource belongs to its account's team,
else to nobody. Name each account's team in `.pudl/schema/aws/owners.cue`, or
in a file of your own in package `aws`:

```cue
_awsAccountOwners: {"123456789012": "platform"}
```

Check findings and `pudl policy show` then name each resource's owner;
`pudl check ... --owner platform` shows one team's findings, and
`pudl query resource_unowned` lists what no account names.

AWS tags are a list of `{Key, Value}` pairs (`Tags`; `TagList` on RDS), and an
owner path names one value by field path: it cannot pick the element whose
`Key` is `Owner` or `Team`. So the pack maps accounts, not tags. To own by tag,
have a collector print the tag as a field, for example
`--query "Vpcs[].merge(@, {OwnerTag: (Tags[?Key=='Owner'].Value)[0]})"`, add
`paths: ["OwnerTag"]` to that type's owner, and regenerate its fixture.

## Policies

Single-resource rules are policies. PUDL evaluates them on every observation
and stores the results as facts (`pudl policy evaluate --json` lists them). A
constrained field that a record lacks makes the result `unknown`, unless the
rule uses `absent="violation"`. The pack uses it only where the collector
leaves a field out to mean "off".

| Policy | Rule | Severity | Absent field |
| --- | --- | --- | --- |
| `#SecurityGroupBestPractice` | `aws-sg-world-sensitive-port`: no ingress from `0.0.0.0/0` or `::/0` that covers a sensitive TCP/UDP port or allows all traffic (`-1`) | high | n/a (the CLI prints every field) |
| `#EC2InstanceBestPractice` | `aws-ec2-imdsv2`: `MetadataOptions.HttpTokens` is `required`, unless the metadata endpoint is disabled | medium | unknown |
| `#EBSVolumeBestPractice` | `aws-ebs-encryption`: `Encrypted` | high | unknown (always printed) |
| `#S3BucketBestPractice` | `aws-s3-block-public-access`: all four bucket-level block public access settings on | high | violation: no bucket-level block |
| | `aws-s3-versioning`: `Versioning.Status` is `Enabled` | medium | violation: never enabled |
| `#RDSInstanceBestPractice` | `aws-rds-storage-encryption`: `StorageEncrypted` | high | unknown (always printed) |
| | `aws-rds-public-access`: `PubliclyAccessible` is false | high | unknown (always printed) |
| | `aws-rds-backups`: `BackupRetentionPeriod` above 0 | medium | unknown (always printed) |
| `#KMSKeyBestPractice` | `aws-kms-rotation`: an enabled, customer managed, symmetric key rotates; reported on `KeyMetadata.KeySpec` | medium | a missing `KeyRotationEnabled` counts as not rotating |
| `#IAMUserBestPractice` | `aws-iam-console-mfa`: a user with a console password (`LoginProfile`) has an MFA device | high | no `LoginProfile` means no console access |
| `#IAMRoleBestPractice` | `aws-iam-role-public-trust`: no Allow statement without a Condition trusts `"*"` or AWS `"*"`; reported on `RoleName` | critical | n/a |

Sensitive ports are the AWS Security Hub EC2.19 list: 20, 21, 22, 23, 25,
110, 135, 143, 445, 1433, 1434, 3000, 3306, 3389, 4333, 5000, 5432, 5500,
5601, 8080, 8088, 8888, 9200 and 9300. A permission's `FromPort`..`ToPort`
range covers a port when it includes it. AWS managed keys rotate on their own
and asymmetric or HMAC keys cannot rotate automatically, so neither is
flagged. A key pending deletion is not flagged for rotation; the
`db-key-unusable` check reports databases that still use it. The
account-level S3 public access block is not collected, so a bucket protected
only by it is still reported.

## Relationships and impact

The schemas project facts (`aws_vpc`, `aws_subnet`, `aws_security_group`,
`aws_instance`, `aws_instance_sg`, `aws_volume`, `aws_volume_attachment`,
`aws_db_instance`, `aws_db_subnet`, `aws_db_sg`, `aws_kms_key`) that join on
AWS IDs and ARNs. The rules relate them:

| Rule | Answers |
| --- | --- |
| `aws_instance_in_vpc(instance, subnet, vpc)` | EC2 instance → subnet → VPC, through the subnet's record |
| `aws_instance_security_group(instance, group, vpc)` | EC2 instance → each attached security group |
| `aws_db_in_vpc(db, subnet_group, vpc)` | RDS instance → DB subnet group → VPC |
| `aws_db_in_subnet(db, subnet, vpc)` | RDS instance → each subnet of its subnet group |
| `aws_uses(resource, kind, uses)` | one direct dependency: subnet and security group → VPC, instance → subnet and group, volume → instance and KMS key, database → VPC, subnet, group and KMS key |
| `aws_depends_on(resource, kind, on)` | the transitive closure of `aws_uses` |
| `aws_depends_on_vpc(vpc, resource, kind)` | everything that depends on a VPC |

What depends on this VPC, or on this security group or key:

```bash
pudl query aws_depends_on_vpc vpc=vpc-0a1b2c3d4e5f60001
pudl query aws_depends_on on=sg-0aa00000000000003
pudl query aws_depends_on on=arn:aws:kms:us-east-1:123456789012:key/0aa00000-0000-4000-8000-000000000003
```

Over the fixtures, the prod VPC has 17 dependents: 3 subnets, 5 security
groups, 3 instances, 4 volumes (through their instances) and 2 databases.
`pudl query` reads latest-known facts, so run the collectors first.

## Check families

Each family is a checks-only model. Its checks read the latest observation of
the named collectors (evidence `model:<collector>`, at most an hour old).

| Family | Check | Severity | Collectors |
| --- | --- | --- | --- |
| `aws-network-exposure-checks` | `policy-high`: any high or critical policy violation | fail | security-groups, instances |
| | `policy-any`: any policy violation | warn | security-groups, instances |
| | `internet-exposed-instance`: a running instance with a public IP in a security group that violates `aws-sg-world-sensitive-port` | fail | instances, security-groups |
| | `instance-in-default-vpc`: an instance whose subnet is in a default VPC | warn | instances, subnets, vpcs |
| `aws-data-protection-checks` | `policy-high`, `policy-any` | fail, warn | s3-buckets, volumes, db-instances, kms-keys |
| | `db-key-unusable`: an RDS instance encrypted with a KMS key that is disabled or pending deletion | fail | db-instances, kms-keys |
| `aws-identity-posture-checks` | `policy-high`, `policy-any` | fail, warn | iam-users, iam-roles |

Name a family together with its collectors; they run first:

```bash
pudl check aws-fixture-vpcs aws-fixture-subnets aws-fixture-security-groups \
  aws-fixture-instances aws-fixture-network-exposure-checks --detailed-exitcode
```

The fixtures contain compliant and noncompliant resources, so every fixture
family exits 2 (a failing fail-severity check). Over the fixtures, network
exposure finds 3 open security groups, 2 instances without IMDSv2, 2
internet-exposed instances and 1 instance in the default VPC; data protection
finds 2 buckets, 2 unencrypted volumes, 1 database (unencrypted, public, no
backups), 1 key without rotation and 1 database on a key pending deletion;
identity posture finds 1 console user without MFA and 2 roles anyone can
assume. `pudl model list` shows every model; each `aws-fixture-*` model has a
live `aws-*` twin.

## Waiving a finding

To accept a finding until a date, add a waiver to the family in
`.pudl/schema/models/aws_pack_checks.cue`. A policy waiver names the policy,
the rule and the resource ID (`pudl policy show aws.#SecurityGroupBestPractice`
lists failing resources by ID):

```cue
#AWSNetworkExposureChecks: _#AWSNetworkExposure & {
	_prefix: "aws-"
	waivers: [{
		policy:   "aws.#SecurityGroupBestPractice"
		rule:     "aws-sg-world-sensitive-port"
		resource: "<resource ID>"
		owner:    "security"
		reason:   "bastion until SSM Session Manager is rolled out"
		created:  "2026-11-01"
		expires:  "2026-12-31"
	}]
}
```

A waiver can instead target a cross-resource check's witness, for example
`{check: "internet-exposed-instance", witness: {instance: "i-0123456789abcdef0"}, ...}`.
The `policy-high` rules carry `severity`, so a waiver covers only the high and
critical violations that check reports.
See [governance](../../docs/governance.md) for how waivers match and expire,
finding changes (`--fail-on new`), owners and CI exit codes.

## Run against live accounts

Do this in a workspace of its own if you want `pudl query` (latest-known
facts) to show only live data.

The live collectors name no account or region. Each lists your AWS CLI
profiles when it runs and repeats its script per profile, with `{{item}}`
replaced by the profile name; the report records the profiles under
`populate.inputs`. A profile can assume a role in another account
(`role_arn`/`source_profile` in `~/.aws/config`), so one set of credentials
can read many accounts. Regions are asked of the profile's configured region,
or `us-east-1` when it has none.

1. Optionally narrow the profiles or regions. Set them in a file of your own,
   so the pack's files stay identical to the bundle, e.g.
   `.pudl/schema/models/aws_settings.cue`:

   ```cue
   package models

   _awsProfileFilter: "*-readonly" // a shell glob; default "*" (all profiles)
   _awsRegionFilter: "us-*"        // a describe-regions name filter; default "*"
   ```

2. Check your credentials and the models:

   ```bash
   aws configure list-profiles
   aws sts get-caller-identity --profile <profile>
   pudl model validate aws-vpcs
   ```

3. Run a family with its collectors, for example:

   ```bash
   pudl check aws-vpcs aws-subnets aws-security-groups aws-instances \
     aws-network-exposure-checks --detailed-exitcode
   pudl policy evaluate --json
   pudl query aws_depends_on_vpc vpc=<a VPC ID from the run>
   ```

A collector's observation is complete only for the profiles it listed and the
regions that matched the filter.

## Limits

- Each record is evaluated on its own. Effective reachability (network ACLs,
  route tables, load balancers, security group references) is not computed;
  `internet-exposed-instance` joins only an instance's public IP to its
  groups' world-open rules.
- Bucket policies and ACLs are not collected; the S3 policy checks the
  bucket-level public access block only.
- IAM policies attached to users and roles are not collected; the IAM
  policies cover console MFA and trust policies only.
- Relationship rules join cataloged resources: an instance's VPC is found
  through its subnet, so collect subnets too.
- AWS CLI output shapes change between releases. Re-check the schemas when
  you upgrade the CLI.

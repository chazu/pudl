# AWS content pack and relationship rules — pudl-xf5.2, pudl-xf5.7

An installable AWS pack written in CUE, the AWS half of the relationship
rules, an impact query for AWS and GCP, and three check families. Installed
with `pudl example install aws`. User docs: `examples/aws/README.md`.

## Packaging

`examples/aws/` mirrors the `.pudl/` layout and is embedded by
`examples/aws/bundle.go` (`//go:embed schema populators`).
`internal/examples.Install` gains an `aws` case over the generic `treeFiles`;
`cmd/example.go` adds `aws` to `ValidArgs`, the help text and the next-step
hint. The Go holds no AWS knowledge.

## Relation to the built-in `pudl/aws` schemas

`pudl init` already installs `pudl/aws.#VPC`, `#Subnet`, `#SecurityGroup` and
other network types. The pack's `aws.#VPC`, `aws.#Subnet` and
`aws.#SecurityGroup` specialize them with `base_schema` and keep their
identity fields (`VpcId`, `SubnetId`, `GroupId`). Reasons: one family root
means one resource ID per VPC whichever schema classified it, so there are
not two conflicting definitions of one resource; a policy on the built-in type
also applies to the pack's; inference prefers the more specific pack type for
collector output (which carries `AccountId`/`Region`) and falls back to the
built-in for plain `describe-*` output. Embedding the built-in definitions
(`awsboot.#VPC & {...}`) is not possible: their `_pudl` would collide. The
pack imports only the built-in component shapes `#Tag` and `#IpPermission`.
`pudl/aws.#Instance` is a normalized snake_case shape (`instance_id`), not
CLI output, so `aws.#EC2Instance` is a separate root; doctor's identity-field
family check passes (asserted in the test).

## Contents

- **Schemas** (package `aws`, all `review: status: "draft"`, all owned):
  `#VPC`, `#Subnet`, `#SecurityGroup` (`network.cue`); `#EC2Instance`,
  `#EBSVolume` (`compute.cue`); `#S3Bucket`, `#RDSInstance`, `#KMSKey`
  (`data.cue`); `#IAMUser`, `#IAMRole` (`iam.cue`). Identity: `VpcId`,
  `SubnetId`, `GroupId`, `InstanceId`, `VolumeId`, `Name` (S3),
  `DBInstanceArn`, `KeyMetadata.Arn`, `User.Arn`, `Arn` (role).
- **Sensitive fields**: `#EC2Instance.UserData`.
- **Fact projection**: `aws_vpc`, `aws_subnet`, `aws_security_group`,
  `aws_instance`, `aws_instance_sg`, `aws_volume`, `aws_volume_attachment`,
  `aws_db_instance`, `aws_db_subnet`, `aws_db_sg`, `aws_kms_key`.
- **Ownership** (`owners.cue`): `_#AWSOwner` maps `AccountId` through
  `_awsAccountOwners` (empty by default). Tag-based owner paths are not
  possible: AWS tags are a `[{Key, Value}]` list and `_pudl.owner.paths` names
  one value by field path, without `[*]` or key selection. The README
  documents the limit and a CUE-only workaround (a JMESPath `merge` in the
  collector's `--query` that lifts a tag into a field).
- **Collectors** (`aws_pack.cue`): `aws-<type>` and `aws-fixture-<type>` for
  vpcs, subnets, security-groups, instances, volumes, db-instances,
  kms-keys, s3-buckets, iam-users, iam-roles. Live runs fan out by
  comprehension over `_awsAccounts` (account ID → CLI profile) ×
  `_awsRegions` (S3 and IAM per account only) and `set` `AccountId` and
  `Region`. List commands use `--query` to unwrap the top-level list
  (`Reservations[].Instances[]` for EC2). Per-resource settings (S3 public
  access block, encryption and versioning; KMS metadata and rotation; IAM
  login profile and MFA) are read by POSIX `sh` scripts held in the model,
  whose `opt` helper leaves a setting out when AWS answers with its "not
  configured" error code and fails the run on any other error. Each collector
  declares a complete observation of `aws/<type>` or `fixture/aws/<type>`.
- **Policies** (rule, severity, absence):
  - `#SecurityGroupBestPractice`: `aws-sg-world-sensitive-port` high — no
    ingress from `0.0.0.0/0`/`::/0` covering a Security Hub EC2.19 port with
    TCP/UDP, or with protocol `-1`.
  - `#EC2InstanceBestPractice`: `aws-ec2-imdsv2` medium (unless the endpoint
    is disabled).
  - `#EBSVolumeBestPractice`: `aws-ebs-encryption` high.
  - `#S3BucketBestPractice`: `aws-s3-block-public-access` high and
    `aws-s3-versioning` medium, both `absent="violation"` (the collector
    leaves them out when not configured).
  - `#RDSInstanceBestPractice`: `aws-rds-storage-encryption` high,
    `aws-rds-public-access` high, `aws-rds-backups` medium.
  - `#KMSKeyBestPractice`: `aws-kms-rotation` medium — enabled customer
    managed keys that do not rotate, reported on the anchor `KeySpec` (must
    not be `SYMMETRIC_DEFAULT`); a missing `KeyRotationEnabled` counts as not
    rotating.
  - `#IAMUserBestPractice`: `aws-iam-console-mfa` high — a `LoginProfile`
    requires a non-empty `MFADevices`.
  - `#IAMRoleBestPractice`: `aws-iam-role-public-trust` critical — an Allow
    statement without a Condition trusting `"*"` or AWS `"*"`, with
    `Statement` as an object or a list; reported on the anchor `RoleName`.
- **Relationship rules (xf5.7)**: `aws_instance_in_vpc` (EC2 → subnet →
  VPC), `aws_instance_security_group` (EC2 → SG), `aws_db_in_vpc` (RDS →
  subnet group → VPC), `aws_db_in_subnet`.
- **Impact**: `aws_uses(resource, kind, uses)` direct edges,
  `aws_depends_on(resource, kind, on)` (recursive closure),
  `aws_depends_on_vpc(vpc, resource, kind)`. GCP equivalent
  `gcp_depends_on_network(network, resource, kind)` over subnets, firewalls,
  routes, routers, GKE clusters, Cloud SQL private networks and private DNS
  zones; this added facts `gcp_network`, `gcp_firewall_network`,
  `gcp_route_network`, `gcp_sql_network`, `gcp_dns_zone`,
  `gcp_dns_zone_network` and a `network` arg on `gcp_subnet`, `gcp_router`
  and `gcp_gke_cluster`. GCP verdicts and policy counts are unchanged.
- **Cross-resource checks**: `aws_instance_internet_exposed` (a running
  instance with a public IP joined to its SGs' `aws-sg-world-sensitive-port`
  policy violations), `aws_instance_in_default_vpc` (built on
  `aws_instance_in_vpc`), `aws_db_key_unusable` (RDS → KMS key Disabled or
  PendingDeletion).
- **Families** (checks-only, `max_age: 1h`): `aws[-fixture]-network-exposure-checks`
  (policy-high, policy-any, internet-exposed-instance fail,
  instance-in-default-vpc warn), `-data-protection-checks` (policy-high,
  policy-any, db-key-unusable), `-identity-posture-checks` (policy-high,
  policy-any). `aws_policy_noncompliant_high` carries `severity` in its head
  for waivers, as the GCP rule does.
- **Fixtures**: 10 files for account `123456789012`, us-east-1; RFC 1918 and
  RFC 5737 addresses, `111122223333` as the partner account, fake key IDs and
  an obviously fake user-data token.

## Tests

`cmd/aws_pack_test.go` `TestAWSContentPack`: install (and an idempotent
reinstall), one `pudl check` of all 10 fixture collectors and 3 families
(exit 2), then:

| Family | Check | Verdict |
| --- | --- | --- |
| network exposure | `policy-high` | fail, 3 (three security groups) |
| | `policy-any` (warn) | fail, 5 (plus two instances without IMDSv2) |
| | `internet-exposed-instance` | fail, 2 (bastion via prod-bastion, legacy-worker via launch-wizard-1) |
| | `instance-in-default-vpc` (warn) | fail, 1 (legacy-worker) |
| data protection | `policy-high` | fail, 5 (two buckets, two volumes, one database) |
| | `policy-any` (warn) | fail, 9 |
| | `db-key-unusable` | fail, 1 (analytics-db, PendingDeletion) |
| identity posture | `policy-high` | fail, 3 (bob; two roles, critical) |
| | `policy-any` (warn) | fail, 3 |

`pudl policy evaluate --json` (compliant/noncompliant/unknown, all
provisional): SecurityGroup 4/3/0, EC2Instance 2/2/0, EBSVolume 4/2/0,
S3Bucket 1/2/0, RDSInstance 2/1/0, KMSKey 4/1/0, IAMUser 2/1/0, IAMRole
3/2/0. Every rule appears in `policy_violation`; a bucket without a public
access block is an `absent` violation. Relationships: `aws_instance_in_vpc` 4,
`aws_instance_security_group` 5, `aws_db_in_vpc` 3, `aws_db_in_subnet` 5;
`aws_depends_on_vpc` for the prod VPC returns 3 subnets, 5 security groups,
3 instances, 4 volumes and 2 databases; `aws_depends_on on=<db SG>` returns
the two databases. `pudl doctor` reports no identity-field inconsistency.
User data is redacted. Every resource is unowned until a test file maps the
account, after which policy findings name `platform` and `resource_unowned`
has no AWS rows. After the noncompliant records are removed, the same check
exits 0 with every check passing.

`cmd/gcp_pack_test.go`: `checkGCPPack` now wraps a shared `checkPack`
(prefix, types, families); witnesses decode `owner`; adds the
`gcp_depends_on_network` counts for prod-vpc (3 subnets, 5 firewalls, 2
routes, 1 router, 2 GKE clusters, 2 SQL instances, 1 DNS zone).

Not committed, run during development: 14 edge records (ICMP and UDP 53 from
the world, TCP 0-65535 from the world, an empty group; a string principal, a
conditioned `AWS: "*"`, a Deny `"*"`, a federated principal; IMDS endpoint
disabled; an instance without MetadataOptions; an AWS managed key and a
customer key without KeyRotationEnabled; a user with MFA but no console)
gave the expected verdicts; and the live collectors (S3, KMS, IAM users and
roles, VPCs) were run through `pudl check` against a stub `aws` on PATH,
producing the expected records and violations. No live AWS run was made.

## Engine findings

None. Policy authoring needed only the documented idioms (annotations outside
comprehensions, `list.Contains` over present values, anchors for conditional
rules). Two design limits, documented rather than worked around in Go:
`#CommandObserve` cannot unwrap a response key (the pack uses the CLI's
`--query`), and owner paths cannot select a tag by key.

## Public API

- `pudl example install aws`.
- `internal/examples`: `Install(root, "aws")`.
- `github.com/chazu/pudl/examples/aws`: `var Files embed.FS`.
- CUE: the schemas, policies, rules and models above; GCP rule
  `gcp_depends_on_network` and its facts.

## Line counts (against `sre-focus` at 6ae1cb7)

- Production Go: +21 / −2: `examples/aws/bundle.go` 8,
  `internal/examples/install.go` +4/−1, `cmd/example.go` +9/−1.
- Test Go: +247 / −7 (`cmd/aws_pack_test.go` 207, `cmd/gcp_pack_test.go`
  +40/−7).
- CUE, AWS pack: +1087 (schemas 375 incl. owners 20, policies 177, rules 215,
  models 320). CUE, GCP impact: +106 / −2.
- Fixtures: +844 lines of JSON. Docs: AWS README 315, GCP README +15, notes,
  cli-reference, docs index, help golden, this log, plan.

## Verification

`CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 mise exec -- make lint` (0
issues), `mise exec -- make check-docs check-skills` pass.

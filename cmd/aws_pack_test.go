package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// awsPackTypes are the collectors of the AWS content pack.
var awsPackTypes = []string{
	"vpcs", "subnets", "security-groups", "instances", "volumes",
	"db-instances", "kms-keys", "s3-buckets", "iam-users", "iam-roles", "iam-instance-profiles",
}

var awsPackFamilies = []string{"network-exposure", "data-protection", "identity-posture"}

func TestAWSContentPack(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "aws", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "pack", "install", "aws", "--json")
	require.NoError(t, r.Err, "identical install is idempotent")

	families := checkPack(t, "aws-fixture-", awsPackTypes, awsPackFamilies, 2)

	ne := families["network-exposure"]
	requireCheck(t, ne, "policy-high", "fail", 3) // three security groups open a sensitive port to the world
	anyViolation := requireCheck(t, ne, "policy-any", "fail", 5)
	require.Equal(t, "warn", anyViolation.Severity, "policy-any only warns")
	exposed := requireCheck(t, ne, "internet-exposed-instance", "fail", 2)
	require.ElementsMatch(t, []string{"i-0aa00000000000002", "i-0dd00000000000001"}, witnessArgs(exposed, "instance"))
	require.ElementsMatch(t, []string{"sg-0aa00000000000002", "sg-0dd00000000000002"}, witnessArgs(exposed, "group"))
	defaultVPC := requireCheck(t, ne, "instance-in-default-vpc", "fail", 1)
	require.Equal(t, "warn", defaultVPC.Severity)
	require.Equal(t, []string{"i-0dd00000000000001"}, witnessArgs(defaultVPC, "instance"))

	dp := families["data-protection"]
	requireCheck(t, dp, "policy-high", "fail", 5) // two buckets, two volumes, one RDS instance
	requireCheck(t, dp, "policy-any", "fail", 9)  // one tuple per resource and severity
	dbKey := requireCheck(t, dp, "db-key-unusable", "fail", 1)
	require.Equal(t, []string{"arn:aws:rds:us-east-1:123456789012:db:analytics-db"}, witnessArgs(dbKey, "db"))
	require.Equal(t, "PendingDeletion", dbKey.Witnesses[0].Arguments["state"])

	ip := families["identity-posture"]
	requireCheck(t, ip, "policy-high", "fail", 3) // bob (high), two roles (critical)
	requireCheck(t, ip, "policy-any", "fail", 3)

	r = runCLI(t, "policy", "evaluate", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var evaluated struct {
		Policies []struct {
			Policy       string `json:"policy"`
			Compliant    int    `json:"compliant"`
			Noncompliant int    `json:"noncompliant"`
			Unknown      int    `json:"unknown"`
			Provisional  int    `json:"provisional"`
		} `json:"policies"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &evaluated), r.Stdout)
	counts := map[string][3]int{}
	for _, p := range evaluated.Policies {
		counts[p.Policy] = [3]int{p.Compliant, p.Noncompliant, p.Unknown}
		require.Equal(t, p.Compliant+p.Noncompliant+p.Unknown, p.Provisional, "draft types make every result provisional")
	}
	require.Equal(t, map[string][3]int{
		"aws.#SecurityGroupBestPractice": {4, 3, 0},
		"aws.#EC2InstanceBestPractice":   {2, 2, 0},
		"aws.#EBSVolumeBestPractice":     {4, 2, 0},
		"aws.#S3BucketBestPractice":      {1, 2, 0},
		"aws.#RDSInstanceBestPractice":   {2, 1, 0},
		"aws.#KMSKeyBestPractice":        {4, 1, 0},
		"aws.#IAMUserBestPractice":       {2, 1, 0},
		"aws.#IAMRoleBestPractice":       {3, 2, 0},
	}, counts)

	r = runCLI(t, "query", "policy_violation")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	for _, rule := range []string{
		"aws-sg-world-sensitive-port", "aws-ec2-imdsv2", "aws-ebs-encryption",
		"aws-s3-block-public-access", "aws-s3-versioning", "aws-rds-storage-encryption",
		"aws-rds-public-access", "aws-rds-backups", "aws-kms-rotation",
		"aws-iam-console-mfa", "aws-iam-role-public-trust",
	} {
		require.Contains(t, r.Stdout, `"rule":"`+rule+`"`)
	}
	// A bucket without a public access block is an absence violation.
	require.Contains(t, r.Stdout, `"kind":"absent","path":"PublicAccessBlockConfiguration.BlockPublicAcls"`)
	require.NotContains(t, r.Stderr, "projection disabled")

	// Relationships: EC2 -> subnet -> VPC, EC2 -> security group, RDS -> subnet group/VPC.
	requireQuery(t, 4, "aws_instance_in_vpc", `"instance":"i-0aa00000000000003","subnet":"subnet-0aa000000000000a2","vpc":"vpc-0a1b2c3d4e5f60001"`)
	requireQuery(t, 5, "aws_instance_security_group", `"group":"sg-0dd00000000000002","instance":"i-0dd00000000000001"`)
	requireQuery(t, 3, "aws_db_in_vpc", `"db":"arn:aws:rds:us-east-1:123456789012:db:reporting-db","subnet_group":"default","vpc":"vpc-0d0d0d0d0d0d0d0d0"`)
	requireQuery(t, 5, "aws_db_in_subnet", `"db":"arn:aws:rds:us-east-1:123456789012:db:orders-db","subnet":"subnet-0aa000000000000a3"`)

	// Impact: what depends on the prod VPC, and on the database security group.
	r = runCLI(t, "query", "aws_depends_on_vpc", "vpc=vpc-0a1b2c3d4e5f60001")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, map[string]int{"subnet": 3, "security_group": 5, "instance": 3, "volume": 4, "db_instance": 2}, kindCounts(r.Stdout))
	require.Contains(t, r.Stdout, `"kind":"volume","resource":"vol-0aa00000000000004"`, "a volume depends on the VPC through its instance and subnet")
	requireQuery(t, 2, "aws_depends_on", "", "on=sg-0aa00000000000003")

	// Packaged types specialize the built-in pudl/aws types without
	// breaking the identity-field family invariant.
	r = runCLI(t, "doctor")
	require.NotContains(t, r.Stdout+r.Stderr, "Inconsistent identity_fields")

	// EC2 user data never reaches storage.
	r = runCLI(t, "list", "--schema", "aws.#EC2Instance", "--select", "UserData", "--all", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "[REDACTED]")
	require.NotContains(t, r.Stdout, "IyEvYmluL3No")

	// Ownership maps accounts to teams. Unmapped, every resource is unowned.
	r = runCLI(t, "query", "resource_unowned")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, `"type":"aws.#IAMRole"`)
	writeFile(t, filepath.Join(pudlDir, "schema", "aws", "owners_local.cue"),
		"package aws\n\n_awsAccountOwners: \"123456789012\": \"platform\"\n")
	families = checkPack(t, "aws-fixture-", awsPackTypes, awsPackFamilies, 2)
	for _, w := range families["data-protection"]["policy-high"].Witnesses {
		require.Equal(t, "platform", w.Owner, "policy findings name the account's team")
	}
	r = runCLI(t, "query", "resource_unowned")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NotContains(t, r.Stdout, `"type":"aws.#`)

	// Remove every noncompliant resource: each family passes.
	dropAWSFixtureRecords(t, pudlDir, map[string][]string{
		"security-groups": {"sg-0aa00000000000002", "sg-0aa00000000000005", "sg-0dd00000000000002"},
		"instances":       {"i-0aa00000000000002", "i-0dd00000000000001"},
		"volumes":         {"vol-0aa00000000000004", "vol-0dd00000000000001"},
		"db-instances":    {"reporting-db", "analytics-db"},
		"kms-keys":        {"0aa00000-0000-4000-8000-000000000004"},
		"s3-buckets":      {"example-public-site", "example-access-logs"},
		"iam-users":       {"bob"},
		"iam-roles":       {"legacy-cross-account", "open-trust-test"},
	})
	families = checkPack(t, "aws-fixture-", awsPackTypes, awsPackFamilies, 0)
	for family, checks := range families {
		for name, c := range checks {
			require.Equal(t, "pass", c.Outcome, "%s %s", family, name)
		}
	}
}

// requireQuery runs `pudl query relation [constraint]` and requires the result
// count and, unless empty, one tuple containing want.
func requireQuery(t *testing.T, count int, relation, want string, constraints ...string) {
	t.Helper()
	r := runCLI(t, append([]string{"query", relation}, constraints...)...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, count, strings.Count(r.Stdout, relation+"("), r.Stdout)
	if want != "" {
		require.Contains(t, r.Stdout, want)
	}
}

// awsRecordKey names an AWS fixture record by the identifier of its type.
func awsRecordKey(rec map[string]any) string {
	for _, key := range []string{"GroupId", "InstanceId", "VolumeId", "DBInstanceIdentifier", "RoleName", "Name"} {
		if v, ok := rec[key].(string); ok {
			return v
		}
	}
	for _, nested := range []string{"KeyMetadata", "User"} {
		if m, ok := rec[nested].(map[string]any); ok {
			for _, key := range []string{"KeyId", "UserName"} {
				if v, ok := m[key].(string); ok {
					return v
				}
			}
		}
	}
	return ""
}

// dropAWSFixtureRecords rewrites installed AWS fixtures without the named
// records.
func dropAWSFixtureRecords(t *testing.T, pudlDir string, drop map[string][]string) {
	t.Helper()
	for typ, keys := range drop {
		path := filepath.Join(pudlDir, "populators", "aws", typ+".json")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var records []map[string]any
		require.NoError(t, json.Unmarshal(data, &records))
		var kept []map[string]any
		for _, rec := range records {
			dropped := false
			for _, k := range keys {
				dropped = dropped || k == awsRecordKey(rec)
			}
			if !dropped {
				kept = append(kept, rec)
			}
		}
		require.Len(t, kept, len(records)-len(keys), typ)
		out, err := json.Marshal(kept)
		require.NoError(t, err)
		writeFile(t, path, string(out))
	}
}

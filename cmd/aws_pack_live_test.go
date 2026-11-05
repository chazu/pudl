package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeAWS answers the AWS CLI commands the AWS pack's live collectors run,
// for three profiles in two regions each, and logs every invocation's
// arguments to $AWS_LOG. A --query ending in merge(@, `{...}`) gets the
// literal's fields added to its record, as JMESPath merge() would.
const fakeAWS = `#!/bin/sh
echo "$*" >> "$AWS_LOG"
prev= q= p= r=
for a in "$@"; do
	case $prev in
	--query) q=$a ;;
	--profile) p=$a ;;
	--region) r=$a ;;
	esac
	prev=$a
done
merged() {
	lit=${q#*\` + "`" + `}
	lit=${lit%\` + "`" + `*}
	printf '[%s,%s]' "${1%\}}" "${lit#\{}"
}
case "$1 $2" in
"configure list-profiles") printf 'dev\nprod-readonly\nsandbox\n' ;;
"configure get")
	[ "$p" = dev ] || exit 1
	echo us-west-2 ;;
"sts get-caller-identity")
	case $p in dev) echo 111111111111 ;; prod-readonly) echo 222222222222 ;; *) echo 333333333333 ;; esac ;;
"ec2 describe-regions") printf 'us-east-1\tus-west-2\n' ;;
"ec2 describe-vpcs")
	merged "{\"VpcId\":\"vpc-$p-$r\",\"OwnerId\":\"$p\",\"CidrBlock\":\"10.0.0.0/16\",\"State\":\"available\",\"IsDefault\":false}" ;;
"iam list-roles")
	merged "{\"Path\":\"/\",\"RoleName\":\"app\",\"RoleId\":\"AROAEXAMPLE\",\"Arn\":\"arn:aws:iam::$p:role/app\",\"CreateDate\":\"2025-11-02T08:55:00+00:00\",\"AssumeRolePolicyDocument\":{\"Version\":\"2012-10-17\",\"Statement\":[]}}" ;;
"s3api list-buckets") printf 'bucket-%s\t2025-11-02T09:00:00+00:00\n' "$p" ;;
"s3api get-public-access-block") echo "An error occurred (NoSuchPublicAccessBlockConfiguration)" >&2; exit 254 ;;
"s3api get-bucket-encryption") echo "An error occurred (ServerSideEncryptionConfigurationNotFoundError)" >&2; exit 254 ;;
"s3api get-bucket-versioning") echo null ;;
*) echo "unexpected aws $*" >&2; exit 1 ;;
esac
`

// The live collectors name no account or region: they fan out over the AWS
// CLI profiles at run time, read each profile's account and enabled regions,
// and filters set outside the pack narrow both.
func TestAWSPackLiveCollectorsEnumerateProfiles(t *testing.T) {
	pudlDir := cliWorkspace(t)
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "aws"), []byte(fakeAWS), 0o755))
	log := filepath.Join(t.TempDir(), "aws.log")
	t.Setenv("AWS_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := runCLI(t, "pack", "install", "aws")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	for model, records := range map[string]int{"aws-vpcs": 6, "aws-iam-roles": 3, "aws-s3-buckets": 3} {
		r = runCLI(t, "check", model, "--json")
		require.NoError(t, r.Err, r.Stdout+r.Stderr)
		report := checkedModel(t, r.Stdout)
		require.NotNil(t, report.Populate, model)
		require.Equal(t, []string{"dev", "prod-readonly", "sandbox"}, report.Populate.Inputs, model)
		require.Empty(t, report.Populate.FailedInputs, model)
		require.Equal(t, records, report.Populate.Records, model)
	}
	r = runCLI(t, "query", "aws_vpc", "vpc_id=vpc-dev-us-west-2", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, `"111111111111"`, "the script merged the profile's account")
	require.Contains(t, r.Stdout, `"us-west-2"`, "and the region")
	calls := readLog(t, log)
	require.Contains(t, calls, "ec2 describe-regions --profile dev --region us-west-2 --filters Name=region-name,Values=*")
	require.Contains(t, calls, "ec2 describe-regions --profile sandbox --region us-east-1")

	writeFile(t, filepath.Join(pudlDir, "schema", "models", "aws_settings.cue"),
		"package models\n\n_awsProfileFilter: \"*-readonly\"\n_awsRegionFilter: \"us-*\"\n")
	r = runCLI(t, "check", "aws-vpcs", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	report := checkedModel(t, r.Stdout)
	require.Equal(t, []string{"prod-readonly"}, report.Populate.Inputs)
	require.Contains(t, readLog(t, log), "--filters Name=region-name,Values=us-*")
}

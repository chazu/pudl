package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeHierarchyGcloud answers the gcloud commands of the GCP pack's
// hierarchy and project-settings collectors for organization 9 (folders 10,
// and 11 inside it) and projects proj-a and proj-b (numbers 1 and 2), and
// logs every invocation to $GCLOUD_LOG. With $GCLOUD_CAI=nobilling every
// asset search fails as with a billing project the caller may not use.
// proj-a set a Binary Authorization policy; proj-b never did, so gcloud
// prints the default and CAI lists nothing.
const fakeHierarchyGcloud = `#!/bin/sh
echo "$*" >> "$GCLOUD_LOG"
for a in "$@"; do
	case $a in
	--project=*) p=${a#--project=} ;;
	--scope=*) scope=${a#--scope=} ;;
	--asset-types=*) at=${a#--asset-types=} ;;
	--organization=*) parent=organizations/${a#--organization=} ;;
	--folder=*) parent=folders/${a#--folder=} ;;
	esac
done
org='{"name":"organizations/9","displayName":"example.com","lifecycleState":"ACTIVE","creationTime":"2016-01-01T00:00:00Z"}'
f10='{"name":"folders/10","parent":"organizations/9","displayName":"prod","lifecycleState":"ACTIVE","createTime":"2019-01-01T00:00:00Z"}'
f11='{"name":"folders/11","parent":"folders/10","displayName":"payments","lifecycleState":"ACTIVE","createTime":"2020-01-01T00:00:00Z"}'
ba='"defaultAdmissionRule":{"evaluationMode":"REQUIRE_ATTESTATION","enforcementMode":"ENFORCED_BLOCK_AND_AUDIT_LOG"}'
crm=//cloudresourcemanager.googleapis.com
case "$1 $2" in
"organizations list") echo "[$org]" ;;
"organizations describe") echo "$org" ;;
"projects list") echo '[{"projectId":"proj-a"},{"projectId":"proj-b"}]' ;;
"asset search-all-resources")
	if [ "$GCLOUD_CAI" = nobilling ]; then
		echo "ERROR: (gcloud.asset.search-all-resources) PERMISSION_DENIED: Caller does not have required permission to use project bill." >&2
		echo "  reason: USER_PROJECT_DENIED" >&2
		exit 1
	fi
	case "$at $scope" in
	"cloudresourcemanager.googleapis.com/Folder organizations/9")
		printf '[{"name":"%s/folders/10","assetType":"%s","parentFullResourceName":"%s/organizations/9","versionedResources":[{"version":"v2","resource":%s}]},' "$crm" "$at" "$crm" "$f10"
		printf '{"name":"%s/folders/11","assetType":"%s","parentFullResourceName":"%s/folders/10","versionedResources":[{"version":"v2","resource":%s}]}]\n' "$crm" "$at" "$crm" "$f11" ;;
	"binaryauthorization.googleapis.com/Policy projects/proj-a")
		printf '[{"name":"//binaryauthorization.googleapis.com/projects/1/policy","assetType":"%s","parentFullResourceName":"%s/projects/1","versionedResources":[{"version":"v1","resource":{"name":"projects/1/policy",%s}}]}]\n' "$at" "$crm" "$ba" ;;
	*) echo '[]' ;;
	esac ;;
"resource-manager folders")
	case $parent in
	organizations/9) echo "[$f10]" ;;
	folders/10) echo "[$f11]" ;;
	*) echo '[]' ;;
	esac ;;
"container binauthz")
	if [ "$p" = proj-a ]; then
		printf '{"name":"projects/proj-a/policy",%s,"etag":"\\"e\\"","updateTime":"2024-01-01T00:00:00Z"}\n' "$ba"
	else
		printf '{"name":"projects/%s/policy","defaultAdmissionRule":{"evaluationMode":"ALWAYS_ALLOW","enforcementMode":"ENFORCED_BLOCK_AND_AUDIT_LOG"},"globalPolicyEvaluationMode":"ENABLE"}\n' "$p"
	fi ;;
*) echo "unexpected gcloud $*" >&2; exit 1 ;;
esac
`

func fakeHierarchyGcloudOnPath(t *testing.T) (log string) {
	t.Helper()
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gcloud"), []byte(fakeHierarchyGcloud), 0o755))
	log = filepath.Join(t.TempDir(), "gcloud.log")
	t.Setenv("GCLOUD_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// listedRecords returns the stored records of a schema.
func listedRecords(t *testing.T, schema string) []map[string]any {
	t.Helper()
	r := runCLI(t, "list", "--json", "--all", "--schema", schema)
	require.NoError(t, r.Err, r.Stderr)
	var out struct {
		Entries []struct {
			StoredPath string `json:"stored_path"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
	var records []map[string]any
	for _, e := range out.Entries {
		raw, err := os.ReadFile(e.StoredPath)
		require.NoError(t, err)
		var rec map[string]any
		require.NoError(t, json.Unmarshal(raw, &rec))
		records = append(records, rec)
	}
	return records
}

// Folders live above projects: even in project mode they are searched per
// organization, stored without a project, and when CAI cannot answer, gcloud
// walks the organization's folders and stores nothing new.
func TestGCPPackFoldersSearchEachOrganization(t *testing.T) {
	pudlDir := cliWorkspace(t)
	log := fakeHierarchyGcloudOnPath(t)
	require.NoError(t, runCLI(t, "pack", "install", "gcp").Err)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"), "package models\n\n_gcpAssetScope: \"projects\"\n")

	r := runCLI(t, "check", "gcp-folders", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr := checkedModel(t, r.Stdout).Populate
	require.Nil(t, pr.Fallback)
	require.Equal(t, []string{"organizations/9"}, pr.Inputs)
	require.Equal(t, 2, pr.Records)
	require.Contains(t, readLog(t, log), "--scope=organizations/9 --asset-types=cloudresourcemanager.googleapis.com/Folder")
	for _, rec := range listedRecords(t, "gcp.#Folder") {
		require.NotContains(t, rec, "project")
	}

	t.Setenv("GCLOUD_CAI", "nobilling")
	r = runCLI(t, "check", "gcp-folders", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr = checkedModel(t, r.Stdout).Populate
	require.NotNil(t, pr.Fallback)
	require.Equal(t, []string{"organizations/9"}, pr.Inputs, "the fallback also runs per organization")
	require.Equal(t, 2, pr.Records, "folder 11 is found inside folder 10")
	require.Zero(t, pr.NewRecords)
	require.Contains(t, readLog(t, log), "resource-manager folders list --folder=10 --format=json")
}

// CAI names a Binary Authorization policy's project by number, so the policy
// is searched per project, whose run names it by ID. gcloud also prints the
// default policy of a project that never set one; the helper drops it, so the
// fallback stores nothing new.
func TestGCPPackBinauthzSearchEachProject(t *testing.T) {
	cliWorkspace(t)
	log := fakeHierarchyGcloudOnPath(t)
	require.NoError(t, runCLI(t, "pack", "install", "gcp").Err)

	r := runCLI(t, "check", "gcp-binauthz-policies", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr := checkedModel(t, r.Stdout).Populate
	require.Nil(t, pr.Fallback)
	require.Equal(t, []string{"proj-a", "proj-b"}, pr.Inputs)
	require.Equal(t, 1, pr.Records)
	require.Contains(t, readLog(t, log), "--scope=projects/proj-a --asset-types=binaryauthorization.googleapis.com/Policy")
	records := listedRecords(t, "gcp.#BinaryAuthorizationPolicy")
	require.Len(t, records, 1)
	require.Equal(t, "proj-a", records[0]["project"])

	t.Setenv("GCLOUD_CAI", "nobilling")
	r = runCLI(t, "check", "gcp-binauthz-policies", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr = checkedModel(t, r.Stdout).Populate
	require.NotNil(t, pr.Fallback)
	require.Equal(t, 1, pr.Records, "proj-b's default policy is dropped")
	require.Zero(t, pr.NewRecords)
}

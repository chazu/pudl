package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeIAMGcloud answers the gcloud commands of the GCP pack's IAM collectors
// for the projects in $GCLOUD_PROJECTS (default proj-a and proj-b) and logs
// every invocation to $GCLOUD_LOG. A project's policy grants roles/viewer to
// a group; the search also finds a public bucket's policy. proj-denied
// refuses both the search and get-iam-policy, as a project without the grant
// does. With $GCLOUD_IAM_SEARCH=nobilling every search fails as with a
// billing project the caller may not use. Each project has one custom role
// (ALPHA, printed by describe only) and one service account with a
// user-managed key; keys list prints only those (--managed-by=user).
const fakeIAMGcloud = `#!/bin/sh
echo "$*" >> "$GCLOUD_LOG"
for a in "$@"; do
	case $a in
	--project=*) p=${a#--project=} ;;
	--scope=projects/*) p=${a#--scope=projects/} ;;
	--format=*) f=${a#--format=} ;;
	--iam-account=*) acct=${a#--iam-account=} ;;
	esac
done
bindings='[{"role":"roles/viewer","members":["group:audit@example.com"]}]'
case "$1 $2 $3" in
"projects list --format=json")
	if [ -n "$GCLOUD_PROJECTS" ]; then
		echo "$GCLOUD_PROJECTS"
	else
		echo '[{"projectId":"proj-a"},{"projectId":"proj-b"}]'
	fi ;;
"asset search-all-iam-policies "*)
	if [ "$GCLOUD_IAM_SEARCH" = nobilling ]; then
		echo "ERROR: (gcloud.asset.search-all-iam-policies) PERMISSION_DENIED: Caller does not have required permission to use project bill." >&2
		echo "  reason: USER_PROJECT_DENIED" >&2
		exit 1
	fi
	if [ "$p" = proj-denied ]; then
		echo "ERROR: (gcloud.asset.search-all-iam-policies) [u@example.com] does not have permission to access projects instance [$p:searchAllIamPolicies] (or it may not exist)" >&2
		exit 1
	fi
	printf '[{"resource":"//cloudresourcemanager.googleapis.com/projects/%s","assetType":"cloudresourcemanager.googleapis.com/Project","project":"projects/1","organization":"organizations/123","policy":{"bindings":%s}},' "$p" "$bindings"
	printf '{"resource":"//storage.googleapis.com/%s-public","assetType":"storage.googleapis.com/Bucket","project":"projects/1","policy":{"bindings":[{"role":"roles/storage.objectViewer","members":["allUsers"]}]}}]\n' "$p" ;;
"projects get-iam-policy "*)
	if [ "$3" = proj-denied ]; then
		echo "ERROR: (gcloud.projects.get-iam-policy) [u@example.com] does not have permission to access projects instance [$3:getIamPolicy] (or it may not exist)" >&2
		exit 1
	fi
	printf '{"bindings":%s,"etag":"BwY=","version":1,"auditConfigs":[{"service":"allServices"}]}\n' "$bindings" ;;
"iam roles list") echo "projects/$p/roles/deployer" ;;
"iam roles describe") printf '{"name":"projects/%s/roles/deployer","title":"Deployer","includedPermissions":["run.services.update"],"stage":"ALPHA","etag":"BwY="}\n' "$p" ;;
"iam service-accounts list") echo "ci@$p.iam.gserviceaccount.com" ;;
"iam service-accounts keys")
	printf '[{"name":"projects/%s/serviceAccounts/%s/keys/abc123","keyType":"USER_MANAGED","keyAlgorithm":"KEY_ALG_RSA_2048","keyOrigin":"GOOGLE_PROVIDED","validAfterTime":"2025-01-01T00:00:00Z","validBeforeTime":"9999-12-31T23:59:59Z"}]\n' "$p" "$acct" ;;
*) echo "unexpected gcloud $*" >&2; exit 1 ;;
esac
`

func fakeIAMGcloudOnPath(t *testing.T) (log string) {
	t.Helper()
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gcloud"), []byte(fakeIAMGcloud), 0o755))
	log = filepath.Join(t.TempDir(), "gcloud.log")
	t.Setenv("GCLOUD_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func snapshotComplete(t *testing.T, id string) bool {
	t.Helper()
	var shown struct {
		Snapshot struct {
			Complete bool `json:"complete"`
		} `json:"snapshot"`
	}
	r := runCLI(t, "snapshot", "show", id, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &shown))
	return shown.Snapshot.Complete
}

// IAM policies are searched per project even in organization mode. A project
// that refuses the search is a failed input, not a reason to fall back: the
// other projects keep their resource-level policies.
func TestGCPPackIAMPoliciesSearchEachProject(t *testing.T) {
	cliWorkspace(t)
	log := fakeIAMGcloudOnPath(t)
	t.Setenv("GCLOUD_PROJECTS", `[{"projectId":"proj-a"},{"projectId":"proj-denied"}]`)
	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "gcp-iam-policies", "--json")
	require.Error(t, r.Err, "a failed input")
	pr := checkedModel(t, r.Stdout).Populate
	require.NotNil(t, pr)
	require.Nil(t, pr.Fallback, "a refused project does not switch sources")
	require.Equal(t, []string{"proj-a", "proj-denied"}, pr.Inputs)
	require.Len(t, pr.FailedInputs, 1)
	require.Equal(t, "proj-denied", pr.FailedInputs[0].Item)
	require.Equal(t, 2, pr.Records, "proj-a's own policy and its bucket's")
	require.Contains(t, readLog(t, log), "asset search-all-iam-policies --scope=projects/proj-a --format=json")
	require.NotContains(t, readLog(t, log), "get-iam-policy")
}

// When the search cannot be billed, gcloud reads each project's own policy,
// in the same shape: nothing new is stored, and because the fallback cannot
// see the buckets' policies its observation is incomplete. "off" does the
// same without trying the search.
func TestGCPPackIAMPoliciesFallBackPartially(t *testing.T) {
	pudlDir := cliWorkspace(t)
	log := fakeIAMGcloudOnPath(t)
	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "gcp-iam-policies", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr := checkedModel(t, r.Stdout).Populate
	require.Nil(t, pr.Fallback)
	require.Equal(t, 4, pr.Records)
	require.True(t, snapshotComplete(t, pr.SnapshotID))

	t.Setenv("GCLOUD_IAM_SEARCH", "nobilling")
	r = runCLI(t, "check", "gcp-iam-policies", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr = checkedModel(t, r.Stdout).Populate
	require.NotNil(t, pr.Fallback)
	require.Equal(t, "USER_PROJECT_DENIED", pr.Fallback.Pattern)
	require.True(t, pr.Fallback.Partial)
	require.Equal(t, 2, pr.Records, "each project's own policy")
	require.Zero(t, pr.NewRecords, "identical to the search's copies")
	require.False(t, snapshotComplete(t, pr.SnapshotID), "the buckets' policies are not marked gone")
	require.Contains(t, readLog(t, log), "projects get-iam-policy proj-b --format=json")

	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"), "package models\n\n_gcpAssetScope: \"off\"\n")
	r = runCLI(t, "check", "gcp-iam-policies", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr = checkedModel(t, r.Stdout).Populate
	require.Nil(t, pr.Fallback)
	require.Equal(t, 2, pr.Records)
	require.False(t, snapshotComplete(t, pr.SnapshotID))
}

// The gcloud sources of custom roles and service account keys are helper
// scripts: each role described (the list omits permissions), each account's
// user-managed keys.
func TestGCPPackIAMRolesAndKeysThroughGcloud(t *testing.T) {
	pudlDir := cliWorkspace(t)
	log := fakeIAMGcloudOnPath(t)
	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"), "package models\n\n_gcpAssetScope: \"off\"\n")

	for _, model := range []string{"gcp-iam-roles", "gcp-service-account-keys"} {
		r = runCLI(t, "check", model, "--json")
		require.NoError(t, r.Err, r.Stdout+r.Stderr)
		pr := checkedModel(t, r.Stdout).Populate
		require.Empty(t, pr.FailedInputs, model)
		require.Equal(t, 2, pr.Records, "%s: one per project", model)
	}
	calls := readLog(t, log)
	require.Contains(t, calls, "iam roles describe deployer --project=proj-a --format=json")
	require.Contains(t, calls, "iam service-accounts keys list --iam-account=ci@proj-b.iam.gserviceaccount.com --managed-by=user --project=proj-b --format=json")

	r = runCLI(t, "query", "gcp_iam_role_permission")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "run.services.update")
}

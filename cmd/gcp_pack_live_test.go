package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeGcloud answers the gcloud commands the GCP pack's live collectors run,
// for two projects, and logs every invocation's arguments to $GCLOUD_LOG.
// $GCLOUD_PROJECTS replaces the project list; in proj-nocompute the Compute
// Engine and KMS APIs are disabled, so those commands fail as gcloud's do
// (KMS without the SERVICE_DISABLED reason); in proj-nobilling every command
// fails with billing disabled.
const fakeGcloud = `#!/bin/sh
echo "$*" >> "$GCLOUD_LOG"
for a in "$@"; do
	case $a in
	--project=*) p=${a#--project=} ;;
	--location=*) l=${a#--location=} ;;
	--keyring=*) r=${a#--keyring=} ;;
	--format=*) f=${a#--format=} ;;
	esac
done
if [ "$1" = compute ] && [ "$p" = proj-nocompute ]; then
	echo "ERROR: (gcloud.$1.$2.list) Compute Engine API has not been used in project $p before or it is disabled." >&2
	echo "- '@type': type.googleapis.com/google.rpc.ErrorInfo" >&2
	echo "  reason: SERVICE_DISABLED" >&2
	exit 1
fi
if [ "$p" = proj-nobilling ] && [ "$1" != projects ]; then
	echo "ERROR: (gcloud.$1.$2.list) FAILED_PRECONDITION: Billing is disabled for project $p. Enable it, then retry." >&2
	exit 1
fi
if [ "$1" = kms ] && [ "$p" = proj-nocompute ]; then
	echo "ERROR: (gcloud.kms.$2.list) PERMISSION_DENIED: Google Cloud KMS API has not been used in project $p before or it is disabled. Enable it, then retry." >&2
	exit 1
fi
case "$1 $2" in
"projects list")
	if [ -n "$GCLOUD_PROJECTS" ]; then
		echo "$GCLOUD_PROJECTS"
	else
		echo '[{"projectId":"proj-a"},{"projectId":"proj-b"}]'
	fi ;;
"compute firewall-rules")
	printf '[{"kind":"compute#firewall","id":"1","creationTimestamp":"2024-02-01T10:00:00.000-08:00","name":"allow-ssh","network":"https://www.googleapis.com/compute/v1/projects/%s/global/networks/default","priority":1000,"sourceRanges":["10.0.0.0/8"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}],"direction":"INGRESS","disabled":false,"selfLink":"https://www.googleapis.com/compute/v1/projects/%s/global/firewalls/allow-ssh"}]' "$p" "$p" ;;
"compute addresses"|"compute routers") echo '[]' ;;
"kms keyrings")
	if [ "$f" = json ]; then
		printf '[{"name":"projects/%s/locations/%s/keyRings/app","createTime":"2024-01-20T10:00:00Z"}]' "$p" "$l"
	else
		echo "projects/$p/locations/$l/keyRings/app"
	fi ;;
"kms keys")
	printf '[{"name":"%s/cryptoKeys/data-key","purpose":"ENCRYPT_DECRYPT","createTime":"2024-01-20T10:05:00Z","rotationPeriod":"7776000s","primary":{"name":"%s/cryptoKeys/data-key/cryptoKeyVersions/1","state":"ENABLED"}}]' "$r" "$r" ;;
*) echo "unexpected gcloud $*" >&2; exit 1 ;;
esac
`

func fakeGcloudOnPath(t *testing.T) (log string) {
	t.Helper()
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gcloud"), []byte(fakeGcloud), 0o755))
	log = filepath.Join(t.TempDir(), "gcloud.log")
	t.Setenv("GCLOUD_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// The live collectors name no project: they fan out over the projects gcloud
// lists at run time, and a filter set outside the pack narrows that list.
func TestGCPPackLiveCollectorsEnumerateProjects(t *testing.T) {
	pudlDir := cliWorkspace(t)
	log := fakeGcloudOnPath(t)
	r := runCLI(t, "example", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	for _, model := range []string{"gcp-firewalls", "gcp-kms-keyrings", "gcp-kms-keys"} {
		r = runCLI(t, "check", model, "--json")
		require.NoError(t, r.Err, r.Stdout+r.Stderr)
		report := checkedModel(t, r.Stdout)
		require.NotNil(t, report.Populate, model)
		require.Equal(t, []string{"proj-a", "proj-b"}, report.Populate.Inputs, model)
		require.Empty(t, report.Populate.FailedInputs, model)
		require.Equal(t, 2, report.Populate.Records, "%s: one record per project", model)
		require.Equal(t, 2, report.Populate.NewRecords, "%s: all new on the first run", model)
	}

	// A re-run over unchanged resources observes the same records but stores
	// none as new; records matches what `pudl snapshot show` reports.
	r = runCLI(t, "check", "gcp-firewalls", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	rerun := checkedModel(t, r.Stdout).Populate
	require.NotNil(t, rerun)
	require.Equal(t, 2, rerun.Records, "the re-run still observed both projects' records")
	require.Zero(t, rerun.NewRecords, "nothing changed, so nothing is stored as new")
	var shown struct {
		Records int `json:"records"`
	}
	r = runCLI(t, "snapshot", "show", rerun.SnapshotID, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &shown))
	require.Equal(t, rerun.Records, shown.Records)

	calls := readLog(t, log)
	require.Contains(t, calls, "projects list --format=json\n")
	require.Contains(t, calls, "compute firewall-rules list --project=proj-b --format=json")
	require.Contains(t, calls, "kms keys list --keyring=projects/proj-a/locations/global/keyRings/app --format=json")

	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"),
		"package models\n\n_gcpProjectFilter: \"labels.env=staging\"\n")
	r = runCLI(t, "check", "gcp-firewalls", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, readLog(t, log), "projects list --format=json --filter=labels.env=staging")
}

// A project with the Compute Engine API disabled has no compute resources:
// the live collectors list it as an empty input, the snapshot stays complete,
// and a family that depends on them is not blocked.
func TestGCPPackLiveCollectorsTreatADisabledAPIAsEmpty(t *testing.T) {
	cliWorkspace(t)
	fakeGcloudOnPath(t)
	t.Setenv("GCLOUD_PROJECTS", `[{"projectId":"proj-a"},{"projectId":"proj-nocompute"}]`)
	r := runCLI(t, "example", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "gcp-firewalls", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr := checkedModel(t, r.Stdout).Populate
	require.NotNil(t, pr)
	require.Equal(t, []string{"proj-a", "proj-nocompute"}, pr.Inputs)
	require.Empty(t, pr.FailedInputs)
	require.Equal(t, 1, pr.Records, "proj-a's rule")
	require.Len(t, pr.EmptyInputs, 1)
	empty := pr.EmptyInputs[0]
	require.Equal(t, "proj-nocompute", empty.Item)
	require.Equal(t, 1, empty.ExitStatus)
	require.Equal(t, "SERVICE_DISABLED", empty.Pattern)
	require.Equal(t, "reason: SERVICE_DISABLED", empty.Reason)

	var shown struct {
		Snapshot struct {
			Complete bool `json:"complete"`
		} `json:"snapshot"`
	}
	r = runCLI(t, "snapshot", "show", pr.SnapshotID, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &shown))
	require.True(t, shown.Snapshot.Complete, "a disabled API is a definite answer, not missing evidence")

	r = runCLI(t, "check", "gcp-firewalls", "gcp-addresses", "gcp-routers", "gcp-network-hygiene-checks", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var check struct {
		Members []struct {
			Model  string `json:"model"`
			Result string `json:"result"`
		} `json:"members"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &check))
	require.Len(t, check.Members, 4)
	for _, m := range check.Members {
		require.Equal(t, "succeeded", m.Result, m.Model)
	}

	// KMS reports a disabled API without SERVICE_DISABLED; the pack's second
	// pattern still makes the project an empty input, through the helper
	// script too.
	for _, model := range []string{"gcp-kms-keyrings", "gcp-kms-keys"} {
		r = runCLI(t, "check", model, "--json")
		require.NoError(t, r.Err, r.Stdout+r.Stderr)
		pr := checkedModel(t, r.Stdout).Populate
		require.Empty(t, pr.FailedInputs, model)
		require.Len(t, pr.EmptyInputs, 1, model)
		require.Equal(t, "proj-nocompute", pr.EmptyInputs[0].Item, model)
		require.Equal(t, "API has not been used in project [^ ]+ before or it is disabled", pr.EmptyInputs[0].Pattern, model)
	}
}

// A billing-disabled project is a failed input by default; a pattern in
// _gcpExtraEmptyWhen, set outside the pack, makes it an empty input.
func TestGCPPackExtraEmptyWhen(t *testing.T) {
	pudlDir := cliWorkspace(t)
	fakeGcloudOnPath(t)
	t.Setenv("GCLOUD_PROJECTS", `[{"projectId":"proj-a"},{"projectId":"proj-nobilling"}]`)
	r := runCLI(t, "example", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "gcp-firewalls", "--json")
	require.Error(t, r.Err)
	pr := checkedModel(t, r.Stdout).Populate
	require.Len(t, pr.FailedInputs, 1)
	require.Equal(t, "proj-nobilling", pr.FailedInputs[0].Item)

	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"),
		"package models\n\n_gcpExtraEmptyWhen: [\"Billing is disabled for project\"]\n")
	r = runCLI(t, "check", "gcp-firewalls", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr = checkedModel(t, r.Stdout).Populate
	require.Empty(t, pr.FailedInputs)
	require.Len(t, pr.EmptyInputs, 1)
	require.Equal(t, "Billing is disabled for project", pr.EmptyInputs[0].Pattern)
}

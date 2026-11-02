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
case "$1 $2" in
"projects list") echo '[{"projectId":"proj-a"},{"projectId":"proj-b"}]' ;;
"compute firewall-rules")
	printf '[{"kind":"compute#firewall","id":"1","creationTimestamp":"2024-02-01T10:00:00.000-08:00","name":"allow-ssh","network":"https://www.googleapis.com/compute/v1/projects/%s/global/networks/default","priority":1000,"sourceRanges":["10.0.0.0/8"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}],"direction":"INGRESS","disabled":false,"selfLink":"https://www.googleapis.com/compute/v1/projects/%s/global/firewalls/allow-ssh"}]' "$p" "$p" ;;
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

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSQLBackupsGcloud answers the gcloud commands of the GCP pack's Cloud
// SQL backup collectors for project proj-a, which has instance db-a and its
// read replica (no backups), and the final backup of a deleted instance,
// gone-db. Backup lists print as many backups as --limit asks for. Every
// invocation is logged to $GCLOUD_LOG.
const fakeSQLBackupsGcloud = `#!/bin/sh
echo "$*" >> "$GCLOUD_LOG"
limit=3
for a in "$@"; do
	case $a in
	--instance=*) instance=${a#--instance=} ;;
	--filter=*) instance=$(echo "${a#--filter=}" | sed 's/^instance="\(.*\)"$/\1/') ;;
	--limit=*) limit=${a#--limit=} ;;
	--format=value*) value=1 ;;
	esac
done
sql=https://sqladmin.googleapis.com/sql/v1beta4/projects/proj-a
case "$1 $2 $3" in
"projects list --format=json") echo '[{"projectId":"proj-a"}]' ;;
"sql instances list") printf 'db-a\ndb-a-replica\n' ;;
"sql backups list")
	if [ -n "$value" ]; then printf 'db-a\ndb-a\ngone-db\n'; exit 0; fi
	case $instance in
	*-replica) echo '[]'; exit 0 ;;
	esac
	sep='['
	i=1
	while [ $i -le "$limit" ]; do
		case $* in
		*--instance=*) printf '%s{"kind":"sql#backupRun","id":"%s","instance":"%s","type":"AUTOMATED","status":"SUCCESSFUL","selfLink":"%s/instances/%s/backupRuns/%s"}' "$sep" "$i" "$instance" "$sql" "$instance" "$i" ;;
		*) printf '%s{"kind":"sql#backup","name":"projects/proj-a/backups/%s-%s","instance":"%s","type":"AUTOMATED","state":"SUCCESSFUL"}' "$sep" "$instance" "$i" "$instance" ;;
		esac
		sep=','
		i=$((i + 1))
	done
	echo ']' ;;
*) echo "unexpected gcloud $*" >&2; exit 1 ;;
esac
`

// Each Cloud SQL backup collector keeps _gcpBackupHistory backups per
// instance: runs per listed instance (a replica has none), backups per
// instance its backups name (a deleted instance's included).
func TestGCPPackSQLBackupsKeepTheNewestPerInstance(t *testing.T) {
	pudlDir := cliWorkspace(t)
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gcloud"), []byte(fakeSQLBackupsGcloud), 0o755))
	log := filepath.Join(t.TempDir(), "gcloud.log")
	t.Setenv("GCLOUD_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	require.NoError(t, runCLI(t, "pack", "install", "gcp").Err)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"), "package models\n\n_gcpBackupHistory: 2\n")

	r := runCLI(t, "check", "gcp-sql-backup-runs", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, 2, checkedModel(t, r.Stdout).Populate.Records)
	for _, rec := range listedRecords(t, "gcp.#SQLBackupRun") {
		require.Equal(t, "db-a", rec["instance"])
		require.Equal(t, "proj-a", rec["project"])
	}
	require.Contains(t, readLog(t, log), "sql backups list --project=proj-a --instance=db-a --limit=2 --format=json")

	r = runCLI(t, "check", "gcp-sql-backups", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, 4, checkedModel(t, r.Stdout).Populate.Records)
	instances := map[any]int{}
	for _, rec := range listedRecords(t, "gcp.#SQLBackup") {
		instances[rec["instance"]]++
	}
	require.Equal(t, map[any]int{"db-a": 2, "gone-db": 2}, instances)
	require.Contains(t, readLog(t, log), `sql backups list --project=proj-a --filter=instance="gone-db" --limit=2 --format=json`)
}

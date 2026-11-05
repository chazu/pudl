#!/bin/sh
# Lists the newest backup runs of every Cloud SQL instance in one project for
# gcp-sql-backup-runs: the project's instances, then each one's newest
# HISTORY backup runs (gcloud lists them newest first). A read replica has
# none. Prints one JSON array per instance; pudl reads the arrays as one
# record list. Any gcloud failure fails the run.
# Usage: sql-backup-runs.sh PROJECT HISTORY
set -eu
project=$1
history=$2
instances=$(gcloud sql instances list --project="$project" --format='value(name)')
for instance in $instances; do
	out=$(gcloud sql backups list --project="$project" --instance="$instance" \
		--limit="$history" --format=json)
	printf '%s\n' "$out"
done

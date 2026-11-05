#!/bin/sh
# Lists the newest backups (the Backup API) of every Cloud SQL instance in one
# project for gcp-sql-backups. Backups outlive their instance, so the
# instances are the ones the project's backups name, not the instance list.
# For each, its newest HISTORY backups (gcloud lists them newest first; the
# filter matches the instance name exactly). Prints one JSON array per
# instance; pudl reads the arrays as one record list. Any gcloud failure
# fails the run.
# Usage: sql-backups.sh PROJECT HISTORY
set -eu
project=$1
history=$2
instances=$(gcloud sql backups list --project="$project" --format='value(instance)')
instances=$(printf '%s\n' "$instances" | sort -u)
for instance in $instances; do
	out=$(gcloud sql backups list --project="$project" \
		--filter="instance=\"$instance\"" --limit="$history" --format=json)
	printf '%s\n' "$out"
done

#!/bin/sh
# Lists every folder in one organization for gcp-folders: the organization's
# folders, then each folder's, breadth first. Prints one JSON array per
# parent; pudl reads the arrays as one record list. Any gcloud failure fails
# the run. Also used by org-policies.sh and firewall-policies.sh.
# Usage: folders.sh organizations/ID
set -eu
# names: the folders/N names in gcloud's JSON output, compact or indented.
names() { tr '{},' '\n\n\n' | sed -n 's/^ *"name": *"\(folders\/[0-9]*\)".*/\1/p'; }
out=$(gcloud resource-manager folders list --organization="${1#organizations/}" --format=json)
printf '%s\n' "$out"
queue=$(printf '%s\n' "$out" | names)
while [ -n "$queue" ]; do
	next=""
	for folder in $queue; do
		out=$(gcloud resource-manager folders list --folder="${folder#folders/}" --format=json)
		printf '%s\n' "$out"
		next="$next $(printf '%s\n' "$out" | names)"
	done
	queue=$(echo $next)
done

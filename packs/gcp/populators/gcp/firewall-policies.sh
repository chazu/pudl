#!/bin/sh
# Lists every hierarchical firewall policy in one organization for
# gcp-firewall-policies: the organization's own, then each folder's (from
# folders.sh). Prints one JSON array per parent. Any gcloud failure fails the
# run.
# Usage: firewall-policies.sh organizations/ID
set -eu
folders=$(sh "$(dirname "$0")/folders.sh" "$1")
gcloud compute firewall-policies list --organization="${1#organizations/}" --format=json
for id in $(printf '%s\n' "$folders" | tr '{},' '\n\n\n' | sed -n 's/^ *"name": *"folders\/\([0-9]*\)".*/\1/p'); do
	gcloud compute firewall-policies list --folder="$id" --format=json
done

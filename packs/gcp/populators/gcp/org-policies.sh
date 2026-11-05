#!/bin/sh
# Lists every organization policy set in one organization for
# gcp-org-policies: on the organization, each of its folders (from
# folders.sh) and each project directly under one of them, each policy
# described (the list omits the rules). Prints one JSON object per policy.
# Any gcloud failure fails the run.
# Usage: org-policies.sh organizations/ID
set -eu
org=${1#organizations/}
folders=$(sh "$(dirname "$0")/folders.sh" "$1")
folders=$(printf '%s\n' "$folders" | tr '{},' '\n\n\n' | sed -n 's/^ *"name": *"folders\/\([0-9]*\)".*/\1/p')
projects=$(gcloud projects list --filter="parent.id:($(echo $org $folders))" --format='value(projectId)')
# describe FLAG: every policy set on the resource FLAG names.
describe() {
	constraints=$(gcloud org-policies list "$1" --format='value(constraint)')
	for c in $constraints; do
		gcloud org-policies describe "${c##*/}" "$1" --format=json
	done
}
describe --organization="$org"
for id in $folders; do
	describe --folder="$id"
done
for p in $projects; do
	describe --project="$p"
done

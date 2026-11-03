#!/bin/sh
# Lists every custom role defined in one project for gcp-iam-roles: the
# project's roles, then each role described (the list omits permissions).
# Prints one JSON object per role; pudl reads them as one record stream. Any
# gcloud failure fails the run.
# Usage: iam-roles.sh PROJECT
set -eu
project=$1
roles=$(gcloud iam roles list --project="$project" --format='value(name)')
for role in $roles; do
	gcloud iam roles describe "${role##*/}" --project="$project" --format=json
done

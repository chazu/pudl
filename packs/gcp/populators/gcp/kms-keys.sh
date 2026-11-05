#!/bin/sh
# Lists every Cloud KMS key in one project for gcp-kms-keys: the key rings in
# each location given, then each ring's keys. Prints one JSON array per ring;
# pudl reads the arrays as one record list. Any gcloud failure fails the run.
# Usage: kms-keys.sh PROJECT LOCATION...
set -eu
project=$1
shift
for location in "$@"; do
	rings=$(gcloud kms keyrings list --project="$project" --location="$location" --format='value(name)')
	for ring in $rings; do
		gcloud kms keys list --keyring="$ring" --format=json
	done
done

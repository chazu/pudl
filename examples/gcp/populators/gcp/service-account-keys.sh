#!/bin/sh
# Lists the user-managed keys of every service account in one project for
# gcp-service-account-keys: key metadata only, never key material. Prints one
# JSON array per account; pudl reads the arrays as one record list. Any
# gcloud failure fails the run.
# Usage: service-account-keys.sh PROJECT
set -eu
project=$1
accounts=$(gcloud iam service-accounts list --project="$project" --format='value(email)')
for account in $accounts; do
	gcloud iam service-accounts keys list --iam-account="$account" --managed-by=user --project="$project" --format=json
done

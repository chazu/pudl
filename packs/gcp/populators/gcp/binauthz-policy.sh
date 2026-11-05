#!/bin/sh
# Prints one project's Binary Authorization policy for
# gcp-binauthz-policies, unless the project never set one: gcloud then
# prints the default policy, which has no updateTime (Cloud Asset Inventory
# lists no policy for such a project). Any gcloud failure fails the run.
# Usage: binauthz-policy.sh PROJECT
set -eu
out=$(gcloud container binauthz policy export --project="$1" --format=json)
case $out in
*'"updateTime"'*) printf '%s\n' "$out" ;;
esac

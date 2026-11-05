#!/bin/sh
# Lists every DNS record set in one project for gcp-dns-record-sets: the
# project's Cloud DNS zones, then each zone's record sets. gcloud prints no
# zone, so each record gets "managedZone" (gcloud's JSON puts each record's
# opening brace alone on a line indented two spaces). Prints one JSON array
# per zone; pudl reads the arrays as one record list. Any gcloud failure
# fails the run.
# Usage: dns-record-sets.sh PROJECT
set -eu
project=$1
zones=$(gcloud dns managed-zones list --project="$project" --format='value(name)')
for zone in $zones; do
	out=$(gcloud dns record-sets list --project="$project" --zone="$zone" --format=json)
	printf '%s\n' "$out" | sed "s/^  {\$/  {\"managedZone\": \"$zone\",/"
done

#!/bin/sh
# Lists every Docker image in one project's Artifact Registry repositories for
# gcp-container-images, with the newest tagged versions of each. Container
# Registry (gcr.io) is served by Artifact Registry repositories named gcr.io,
# us.gcr.io and so on, so they are listed here too. Remote (pull-through
# cache) and virtual repositories are skipped: their images live elsewhere.
#
# Prints one JSON object per image: the repository's full name, the image's
# package path and its newest HISTORY tagged versions, newest push first, as
# `gcloud artifacts docker images list` prints them. pudl reads the objects
# as one record stream. Untagged versions are left out. gcloud reads every
# version of an image to sort them; the largest page size (1000) makes that
# a few seconds for an image with thousands of versions, against tens of
# seconds at the default. Any gcloud failure fails the run.
# Usage: container-images.sh PROJECT HISTORY
set -eu
project=$1
history=$2
# Location and repository ID, in pairs.
repos=$(gcloud artifacts repositories list --project="$project" \
	--filter='format=DOCKER AND mode=STANDARD_REPOSITORY' \
	--format='value(name.segment(3),name.segment(5))')
set -- $repos
while [ $# -ge 2 ]; do
	location=$1
	repo=$2
	shift 2
	packages=$(gcloud artifacts packages list --project="$project" \
		--location="$location" --repository="$repo" --page-size=1000 --format='value(name)')
	for package in $packages; do
		# Nested image names come back with / escaped.
		image="$location-docker.pkg.dev/$project/$repo/$(printf '%s' "$package" | sed 's/%2F/\//g')"
		printf '{"repository":"projects/%s/locations/%s/repositories/%s","package":"%s","versions":' \
			"$project" "$location" "$repo" "$image"
		# The package filter keeps out images nested below this one.
		gcloud artifacts docker images list "$image" --include-tags \
			--filter="tags:* AND package=\"$image\"" --sort-by=~CREATE_TIME \
			--limit="$history" --page-size=1000 --format=json
		printf '}\n'
	done
done

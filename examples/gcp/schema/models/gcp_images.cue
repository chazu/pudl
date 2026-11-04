package models

import "regexp"

// Container images (gcp-container-images): every Docker image in each
// project's Artifact Registry repositories, gcr.io repositories included,
// with its newest _gcpImageHistory tagged versions (gcp_pack.cue). gcloud
// only: Cloud Asset Inventory lists every image version as its own asset
// (tens of thousands in a large organization) and cannot keep only the newest
// per image. The helper script (populators/gcp/container-images.sh) lists the
// project's repositories, their images, then each image's versions.

// Listing an image's versions reads all of them (gcloud sorts on the
// client). A project with many images, some with many
// versions, can outlast the default 10-minute run timeout.
_gcpRunTimeout: "container-images": "30m"

// The helper prints {repository, package, versions} per image, versions as
// `gcloud artifacts docker images list --include-tags` prints them. The
// record keeps each version's digest, tags, push time (createTime), build
// time, size and media type; updateTime is dropped (a repository migration
// or a moved tag changes it), as is the version's resource name.
_gcpShapes: "container-images": {
	in: {...}
	let r = in
	out: {
		name:       r.package
		repository: r.repository
		location:   regexp.FindSubmatch("/locations/([^/]+)/", r.repository)[1]
		if len(r.versions) > 0 {
			digest: r.versions[0].version
		}
		versions: [for v in r.versions {
			digest:   v.version
			tags:     v.tags
			pushTime: v.createTime
			if v.metadata.buildTime != _|_ {
				buildTime: v.metadata.buildTime
			}
			if v.metadata.imageSizeBytes != _|_ {
				sizeBytes: v.metadata.imageSizeBytes
			}
			if v.metadata.mediaType != _|_ {
				mediaType: v.metadata.mediaType
			}
		}]
	}
}

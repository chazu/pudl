package gcp

// Container images in Artifact Registry Docker repositories, including the
// gcr.io repositories that now serve Container Registry. One record per
// image, not per version: an image's history can run to thousands of
// versions, so a record keeps only the newest tagged ones (newest push
// first, at most _gcpImageHistory, default 10, in the models package).
// Untagged versions are not kept. Identity is the image path,
// LOCATION-docker.pkg.dev/PROJECT/REPOSITORY/IMAGE. digest is the newest
// tagged version's; an image with no tagged version has none.
//
// Nothing here is secret: the fields are names, digests, tags, times and
// sizes, so no field is redacted.
#ContainerImage: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.artifactregistry.docker_image"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {
			name: {path: "name", last_segment: true}
			region: "location"
		}
		graph: display: {code: "IMG", label: "Container image", class: "compute"}
		facts: {
			gcp_container_image: args: {repository: "repository", location: "location"}
			gcp_container_image_tag: {
				each: "versions[*]"
				args: {tag: "tags[*]", digest: "digest", push_time: "pushTime"}
			}
		}
	}
	project:    _gcpProjectID
	name:       =~"^[a-z0-9-]+-docker\\.pkg\\.dev/[^/]+/[^/]+/.+$" @pudl(match)
	repository: =~"^projects/[^/]+/locations/[^/]+/repositories/[^/]+$"
	location:   string
	digest?:    =~"^sha256:[0-9a-f]{64}$"
	versions: [...#ContainerImageVersion]
	...
}

// One tagged version of an image. pushTime is when the version was pushed
// (Artifact Registry's createTime; its updateTime moves when a repository is
// migrated or a tag is moved, so it is not kept).
#ContainerImageVersion: {
	digest:     =~"^sha256:[0-9a-f]{64}$"
	tags:       [string, ...string]
	pushTime:   string
	buildTime?: string
	sizeBytes?: string
	mediaType?: string
	...
}

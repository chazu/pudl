package artifact

// ImageRef represents a container image reference with digest pinning.
// The digest field defaults to empty until the image is locked.
#ImageRef: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "artifact.image"
		identity_fields: ["source", "tag"]
		tracked_fields: ["digest"]
	}

	source: string       // upstream registry/repo e.g. "ghcr.io/org/image"
	tag:    string       // version tag e.g. "v1.2.3"
	digest: string | *"" // sha256 digest, empty until locked
}

// ArtifactRef represents a generic content-addressed artifact.
#ArtifactRef: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "artifact"
		identity_fields: ["name"]
		tracked_fields: ["version", "sha256"]
	}

	name:    string
	version: string
	sha256?: string
	url?:    string
	...
}

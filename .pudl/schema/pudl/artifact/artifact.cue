package artifact

// ImageRef represents a container image reference with digest pinning.
// The digest field defaults to empty until the image is locked.
#ImageRef: {
	_pudl: {
		schema_type:   "base"
		resource_type: "artifact.image"
		identity_fields: ["source", "tag"]
		tracked_fields: ["digest"]
	}

	source: string       @pudl(binding=plain) // upstream registry/repo e.g. "ghcr.io/org/image"
	tag:    string       @pudl(binding=plain) // version tag e.g. "v1.2.3"
	digest: string | *"" @pudl(binding=plain) // sha256 digest, empty until locked
}

// ArtifactRef represents a generic content-addressed artifact.
#ArtifactRef: {
	_pudl: {
		schema_type:   "base"
		resource_type: "artifact"
		identity_fields: ["name"]
		tracked_fields: ["version", "sha256"]
	}

	name:    string @pudl(binding=plain)
	version: string @pudl(binding=plain)
	sha256?: string @pudl(binding=plain)
	url?:    string @pudl(binding=plain)
	...
}

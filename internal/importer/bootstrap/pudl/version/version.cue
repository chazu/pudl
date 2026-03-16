package version

// Version represents a pinned version string with optional constraint.
#Version: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "version"
		identity_fields: ["name"]
		tracked_fields: ["version"]
	}

	name:        string
	version:     string
	constraint?: string // e.g. "LTS only", "must match upstream"
}

// ToolVersion extends Version with download and verification metadata.
#ToolVersion: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "version.tool"
		identity_fields: ["name"]
		tracked_fields: ["version", "sha256"]
	}

	name:        string
	version:     string
	constraint?: string
	url?:        string
	sha256?:     string
}

// SyncTarget describes a file that must contain a version string.
// Used for tracking where versions are referenced across a project.
#SyncTarget: {
	file:    string // path relative to project root
	pattern: string // how the version appears in the file
	note?:   string
}

// TrackedVersion is a version with sync chain metadata.
// Single source of truth: update the version here, sync targets tell you
// what else needs updating.
#TrackedVersion: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "version.tracked"
		identity_fields: ["name"]
		tracked_fields: ["version"]
	}

	name:        string
	version:     string
	constraint?: string
	sync: [...#SyncTarget]
}

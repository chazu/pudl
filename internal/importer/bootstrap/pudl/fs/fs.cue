package fs

// File represents a filesystem entry with type and permissions.
#File: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "fs.file"
		identity_fields: ["path"]
		tracked_fields: ["mode", "size"]
	}

	path:    string
	type:    "file" | "symlink" | "directory"
	mode:    string | *"644"
	size?:   int & >=0
	target?: string // symlink target
}

// Dir represents a directory with typed file and subdirectory maps.
// Use close({}) on the maps for exhaustive validation.
#Dir: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "fs.dir"
		identity_fields: ["path"]
		tracked_fields: []
	}

	path:   string
	files?: {[string]: #File}
	dirs?:  {[string]: #Dir}
}

// Layout describes an expected directory structure for validation.
// Intended for use with close({}) to catch unexpected files.
#Layout: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "fs.layout"
		identity_fields: ["name"]
		tracked_fields: []
	}

	name:        string
	description?: string
	root:        #Dir
}

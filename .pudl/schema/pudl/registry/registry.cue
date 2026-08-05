package registry

// Entry is the base type for any inventory item.
// Extend it for domain-specific registries:
//   #MyThing: { name: string, description?: string, custom_field: int }
#Entry: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "registry.entry"
		identity_fields: ["name"]
		tracked_fields: []
	}

	name:        string
	description?: string
}

// Domain represents a registered domain name.
#Domain: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "registry.domain"
		identity_fields: ["name"]
		tracked_fields: []
	}

	name:        string
	description?: string
}

// Formatter describes a code formatting tool.
#Formatter: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "registry.formatter"
		identity_fields: ["name"]
		tracked_fields: ["version"]
	}

	name:    string
	description?: string
	tool:    string       // tool binary name
	version: string       // resolved version
	cmd: [...string]      // command args after the binary
	extensions: [...string] // file extensions this handles
	runtime: *"native" | "jar"
}

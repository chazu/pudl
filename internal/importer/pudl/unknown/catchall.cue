package unknown

// CatchAll schema for unclassified data
#CatchAll: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type:      "catchall"
		resource_type:    "unknown"
		cascade_priority: 0
		identity_fields: []
		tracked_fields: []
		compliance_level: "permissive"
	}

	// Accept any structure
	...
}

package core

// ObserveResult is the schema for an observed record that names no schema: the
// collector has no `schema:`, the record carries no `_schema` and claims no
// type by @pudl(match). Formerly pudl/mu.#ObserveResult; that name still
// resolves here (schemaname.Normalize), as does pudl/mu.#ObserveSnapshot.
#ObserveResult: {
	_pudl: {
		schema_type:   "base"
		resource_type: "pudl.observe"
		identity_fields: ["target", "current"]
	}

	target?: string
	current?: {...}
	error?: string
	...
}

// ObserveSnapshot is one observation: the collection of all records observed
// across one or more targets at a point in time, created by each populate run.
// Formerly pudl/mu.#ObserveSnapshot.
#ObserveSnapshot: {
	_pudl: {
		schema_type:   "collection"
		resource_type: "pudl.observe_snapshot"
		identity_fields: ["snapshot_id"]
	}

	snapshot_id: string // timestamp-based ID
	timestamp:   string // ISO 8601
	origin:      string
	run_id?:     string // enclosing pudl run, when available
	targets: [...string] // targets that were observed
	record_count:        int & >=0 // total records across all targets
	schema_summary: [...{// distribution of _schema types
		schema: string
		count:  int & >=0
	}]
	errors?: [...{// targets that reported errors
		target: string
		error:  string
	}]
}

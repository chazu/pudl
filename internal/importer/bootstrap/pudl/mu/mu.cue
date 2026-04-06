package mu

// Manifest represents the output of `mu build --emit-manifest`.
// Each build produces one manifest documenting what actions were executed,
// which were cached, and what artifacts were produced.
#Manifest: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "mu.manifest"
		identity_fields: ["timestamp"]
		tracked_fields:  ["summary", "actions", "targets"]
	}

	version:    int & >=1
	type:       "mu.build.manifest/v1"
	timestamp:  string // ISO 8601
	duration_s: number & >=0
	targets: [...#ManifestTarget]
	actions: [...#ManifestAction]
	summary: {
		completed: int & >=0
		cached:    int & >=0
		failed:    int & >=0
		cancelled: int & >=0
	}
}

// ManifestTarget describes a target that was part of the build.
// Carries BRICK classification metadata for round-tripping through pudl.
#ManifestTarget: {
	name:        string
	toolchain:   string
	kind?:       string // BRICK kind (set by pudl export-actions)
	implements?: string // BRICK interface (set by pudl export-actions)
}

// ManifestAction records the outcome of a single action in a build.
#ManifestAction: {
	id:        string
	cached:    bool
	exit_code: int
	outputs: {[string]: string} | *{}
}

// ObserveResult represents a single observe record that doesn't declare a
// _schema field. Records with _schema are routed to their specific schema
// (e.g. pudl/linux.#Host); this is the fallback for untyped observe data.
#ObserveResult: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "mu.observe"
		identity_fields: ["target"]
		tracked_fields:  []
	}

	target?: string
	...
}

// ObserveSnapshot represents a single mu observe run — the collection of all
// records observed across one or more targets at a point in time.
// Created by `pudl ingest-observe` to group records from one invocation.
#ObserveSnapshot: {
	_pudl: {
		schema_type:     "collection"
		resource_type:   "mu.observe_snapshot"
		identity_fields: ["snapshot_id"]
		tracked_fields:  ["targets", "record_count", "schema_summary"]
	}

	snapshot_id:  string             // timestamp-based ID
	timestamp:    string             // ISO 8601
	origin:       string             // e.g. "mu-observe"
	targets:      [...string]        // targets that were observed
	record_count: int & >=0          // total records across all targets
	schema_summary: [...{            // distribution of _schema types
		schema: string
		count:  int & >=0
	}]
	errors?: [...{                   // targets that reported errors
		target: string
		error:  string
	}]
}

// PlanOutput represents the output of `mu build --plan --json`.
#PlanOutput: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "mu.plan"
		identity_fields: ["targets"]
		tracked_fields:  ["actions"]
	}

	version: int & >=1
	targets: [...string]
	actions: [...#PlanAction]
	summary: {
		total: int & >=0
	}
}

// PlanAction describes a planned action before execution.
#PlanAction: {
	id:        string
	command:   [...string]
	inputs:    {[string]: string} | *{}
	outputs:   [...string] | *[]
	depends_on: [...string] | *[]
	env?:      {[string]: string}
	network?:  bool
	work_dir?: string
}

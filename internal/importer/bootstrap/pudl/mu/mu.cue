package mu

// Manifest represents the output of `mu build --emit-manifest`.
// Each build produces one manifest documenting what actions were executed,
// which were cached, and what artifacts were produced.
#Manifest: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "mu.manifest"
		identity_fields: ["timestamp"]
		tracked_fields:  ["summary", "actions"]
	}

	version:    int & >=1
	type:       "mu.build.manifest/v1"
	timestamp:  string // ISO 8601
	duration_s: number & >=0
	actions: [...#ManifestAction]
	summary: {
		completed: int & >=0
		cached:    int & >=0
		failed:    int & >=0
		cancelled: int & >=0
	}
}

// ManifestAction records the outcome of a single action in a build.
#ManifestAction: {
	id:        string
	cached:    bool
	exit_code: int
	outputs: {[string]: string} | *{}
}

// ObserveResult represents a single entry from `mu observe --json`.
// The output is an array of these, one per observed target.
#ObserveResult: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "mu.observe"
		identity_fields: ["target"]
		tracked_fields:  ["state", "diff"]
	}

	target: string                         // e.g. "//k8s/api-deployment"
	state:  "converged" | "drifted" | "unknown"
	diff?:  string                         // human-readable diff when drifted
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

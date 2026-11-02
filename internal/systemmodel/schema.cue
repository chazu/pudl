package systemmodel

// #SystemModel — a packaging of the IDEA layers / ACUTE phases behind one
// declaration. pudl owns this schema; `pudl check <model>` loads an *instance*
// of it (the run unit) and orchestrates its phases.
//
// populate is #CommandObserve. A model without populate is checks-only: it
// evaluates checks over what is already in the catalog. PUDL observes and
// reports; it never changes the systems it catalogs.
#SystemModel: {
	// Catalog classification: a #SystemModel instance is itself a catalog
	// resource (so `pudl schema list` shows the schema and instances can be
	// inventoried). Hidden + concrete, so loading/validating an instance never
	// has to supply it.
	_pudl: {
		schema_type:   "base"
		resource_type: "system_model"
		identity_fields: ["name"]
		tracked_fields: ["populate", "desired", "checks", "depends_on"]
	}

	name: string
 observation?: {scope: string & !="", complete: bool | *false, schemas?: [...string]}
	// Quoted because this is the JSON-visible routing tag emitted when the
	// model itself is persisted as a typed catalog resource.
	"_schema"?: string

	// DEPENDS_ON — NAMES of other #SystemModel instances whose output this
	// model's desired/observed state depends on. Model names (each dependency's
	// `name:` field), not value references: this expresses ordering/impact, not
	// value interpolation, and not Datalog rule references (that is
	// `relations?`). Emitted as `model_depends_on(from,to)` facts on every run;
	// query the shipped recursive rules `depends_transitive` / `impacted_by` /
	// `cyclic`. See docs/cross-model-dependencies.md.
	depends_on?: [...string]

	// schema — definition references the model's records bind to (validation /
	// catalog binding). Opaque to orchestration; carried, not interpreted here.
	schema?: [...]

	// POPULATE — Accumulate: fetch the external system into the catalog.
	// Optional: without it the model is checks-only (no desired). The retired
	// arms are accepted only so the loader can reject them by name.
	populate?: #CommandObserve | #PluginObserve | #EweTarget

	// RELATE — derived relationships (pudl Datalog rule references).
	relations?: [...string]

	// CHECK — the observe-only flag queries (pudl Datalog).
	checks?: [...#Check]

	// DESIRED — declared desired state (IDEA Definition layer). Present → each
	// run reports drift between it and what was observed; nothing is changed.
	desired?: [...#DesiredResource]

	// RETIRED — accepted here only so a model that still declares them
	// compiles; the loader rejects them with an error naming the field
	// (docs/retired-commands.md).
	plugins?:  [...#PluginDef]
	converge?: #PluginPlan
	inputs?: {...}
	bindings?: {[string]: #ValueBinding}

	// FRESHNESS — loop cadence.
	freshness?: #Freshness
}

// #CommandObserve — run plain commands that print JSON records (gcloud, kubectl,
// aws … --format=json) and ingest their output. Fan out with a CUE
// comprehension:
//
//	runs: [for p in _projects {argv: ["gcloud", "compute", "firewall-rules",
//	    "list", "--project=\(p)", "--format=json"], set: project: p}]
//
// Each run's stdout is a JSON array of records, or a stream of JSON objects.
// Any failing run fails the populate phase and nothing is ingested. argv and
// set are recorded with the model instance: keep secrets out of them.
//
// A record may self-tag with a QUOTED "_schema" label (e.g.
// {"_schema": "git.repository", ...}) to route it. The quote matters in CUE —
// a bare _schema is a hidden field and is dropped from JSON.
#CommandObserve: {
	runs: [#CommandRun, ...#CommandRun]
	// schema routes every record like strict `pudl import --schema`.
	// allow_schema_fallback explicitly permits exploratory base/catchall fallback.
	schema?: string
 allow_schema_fallback?: bool | *false
	// timeout applies to each run (Go duration).
	timeout?: string
}

// #CommandRun — one command. argv is executed directly (no shell).
#CommandRun: {
	argv: [string, ...string]
	// set assigns fields on every record this run prints, overwriting them
	// (nested structs set leaf fields), e.g. the project gcloud omits.
	set?: {...}
	// dir is the working directory, relative to the model file.
	dir?: string
}

// RETIRED shapes (docs/retired-commands.md). Kept so model files that still
// reference them compile and the loader can reject them with an error naming
// the field. Each is closed, so the populate disjunction stays unambiguous.
#PluginDef: {...}

#PluginObserve: {
	plugin:         string
	input?:         {...}
	differential?:  bool
	sealed_inputs?: _
}

#EweTarget: {
	eweSource:      string
	outputs?:       [...string]
	network?:       bool
	impure?:        bool
	sealed_inputs?: _
}

#PluginPlan: {
	plugin:          string
	input?:          {...}
	sealed_inputs?:  _
	sealed_outputs?: _
}

// #ValueBinding is the retired value-binding shape, kept so old model files
// compile and are rejected by name.
#ValueBinding: {
	source: #ResourceRef
	path:   string & !=""
}

#ResourceRef: {
	model:  string & !=""
	schema: string & !=""
	identity: {[string]: _}
}

// #DesiredResource — one declared resource. Open: the fields a resource carries
// belong to its own schema, not to #SystemModel, so anything is allowed through.
#DesiredResource: {...}

// #Check — an observe-only flag: evaluate a Datalog relation, assert empty /
// nonempty, attach a severity.
#Check: {
 evidence?: [...string] // snapshot IDs, "current", "model:<name>", or "scope:<name>"
 max_age?: string // maximum selected observation age, Go duration
	name:     string
	query:    string
	expect:   "empty" | "nonempty"
	severity: "info" | "warn" | "fail"
	message:  string
}

// #Freshness — how the model stays current.
#Freshness: {
	every?: string
	drift?: bool | *false
}

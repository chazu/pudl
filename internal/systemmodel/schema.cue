package systemmodel

// #SystemModel — a packaging of the IDEA layers / ACUTE phases behind one
// declaration. pudl owns this schema; `pudl run <model>` loads an *instance*
// of it (the run unit) and orchestrates its phases. See
// docs/design/system-models/V1-BUILD-SPEC.md.
//
// V1-narrowed: populate is #PluginObserve | #EweTarget | #CommandObserve. A
// model without populate is checks-only: it evaluates checks over what is
// already in the catalog. PUDL observes and reports; it never changes the
// systems it catalogs.
#SystemModel: {
	// Catalog classification: a #SystemModel instance is itself a catalog
	// resource (so `pudl schema list` shows the schema and instances can be
	// inventoried). Hidden + concrete, so loading/validating an instance never
	// has to supply it.
	_pudl: {
		schema_type:   "base"
		resource_type: "system_model"
		identity_fields: ["name"]
		tracked_fields: ["populate", "desired", "checks", "plugins", "depends_on"]
	}

	name: string
 observation?: {scope: string & !="", complete: bool | *false, schemas?: [...string]}
	// Quoted because this is the JSON-visible routing tag emitted when the
	// model itself is persisted as a typed catalog resource.
	"_schema"?: string

	// INPUTS + BINDINGS are authoring-only. PUDL resolves every binding to one
	// concrete scalar and unifies it into inputs before decoding SystemModel.
	// Runtime models and catalog model-instance records omit both fields.
	inputs?: {...}
	bindings?: {[string]: #ValueBinding}

	// DEPENDS_ON — NAMES of other #SystemModel instances whose output this
	// model's desired/observed state depends on. Model names (each dependency's
	// `name:` field), not value references: this expresses ordering/impact, not
	// value interpolation (Terraform-style ${vpc.id} stays parked behind
	// ewe-converge), and not Datalog rule references (that is `relations?`).
	// Emitted as `model_depends_on(from,to)` facts on every run; query the
	// shipped recursive rules `depends_transitive` / `impacted_by` / `cyclic`.
	// See docs/cross-model-dependencies.md.
	depends_on?: [...string]

	// PLUGINS — the plugins this model's arms reference, declared mu-natively so
	// the model is self-contained. Mirrors mu.cue's `plugins:` (#PluginDef): an
	// arm names a plugin (`plugin: "k8s"`), this block says where it comes from.
	// pudl passes these straight into the generated mu.cue. Declare once; reuse
	// across populate + converge.
	plugins?: [...#PluginDef]

	// schema — definition references the model's records bind to (validation /
	// catalog binding). Opaque to orchestration; carried, not interpreted here.
	schema?: [...]

	// POPULATE — Accumulate: fetch the external system into the catalog.
	// Optional: without it the model is checks-only (no desired).
	populate?: #PluginObserve | #EweTarget | #CommandObserve

	// RELATE — derived relationships (pudl Datalog rule references).
	relations?: [...string]

	// CHECK — the observe-only flag queries (pudl Datalog).
	checks?: [...#Check]

	// DESIRED — declared desired state (IDEA Definition layer). Present → each
	// run reports drift between it and what was observed; nothing is changed.
	desired?: [...#DesiredResource]

	// RETIRED — accepted here only so a model that still declares it compiles;
	// the loader rejects it with an error naming the field
	// (docs/retired-commands.md).
	converge?: #PluginPlan

	// FRESHNESS — loop cadence.
	freshness?: #Freshness
}

// #PluginDef — a plugin source, mirroring mu's #PluginDef forms. `name` matches
// what an arm references via `plugin:`; one of command (run this argv directly),
// script (local source, built/cached), digest (from the ~/.mu/plugins CAS
// cache), or url+sha256 (remote) says where it comes from. pudl emits these
// verbatim into the generated mu.cue.
#PluginDef: {
	name: string & !=""
	command?: [...string] // mutable escape hatch; guarded exact mutation rejects zero-digest command plugins
	script?:              string // local .bb / binary path (relative to the model dir, or abs)
	digest?:              string // content digest, resolved from the mu plugin cache
	url?:                 string // remote bundle
	sha256?:              string // required with url
}

// #PluginObserve — reuse a shipped observer plugin (the `host`/`k8s` case). Its
// observe op runs and its output is ingested as the live side. `plugin` names a
// #PluginDef declared in the model's `plugins:` block.
#PluginObserve: {
	sealed_inputs?: _ // retired; rejected by the loader
	plugin:         string
	input: {...}
	// differential: this observer reads `desired` as sources and reports
	// per-resource exists/matches (the k8s case) — drift is interpreted from its
	// output. false → inventory: the observer dumps current records and pudl
	// set-diffs them against `desired` from the catalog. Default true preserves the
	// k8s differential path; inventory observers (e.g. host) set false so a plain
	// `pudl run` routes to inventory drift without needing --from-catalog.
	differential: bool | *true
}

// #EweTarget — a custom ewe fetch program (the GitLab case). See
// ewe-populate-spec.md. `pudl run` renders a mu target with an inline plan that
// emits an `ewe`-body action, runs `mu build`, then wraps each declared output
// (a records array) as an ObserveResult and ingests it (ewe-populate-spec §3).
//
// Convention: each emitted record self-tags with a QUOTED "_schema" label
// (e.g. {"_schema": "git.repository.gitlab", ...}). The quote matters — a bare
// _schema is a hidden CUE field and json.Marshal drops it, so the routing tag
// would never reach the records file.
#EweTarget: {
	sealed_inputs?: _ // retired; rejected by the loader
	eweSource:      string
	outputs: [...string]
	network?: bool | *false
	impure?:  bool | *false
}

// #CommandObserve — run plain commands that print JSON records (gcloud, kubectl,
// aws … --format=json) and ingest their output, with no plugin protocol and no
// mu. Fan out with a CUE comprehension:
//
//	runs: [for p in _projects {argv: ["gcloud", "compute", "firewall-rules",
//	    "list", "--project=\(p)", "--format=json"], set: project: p}]
//
// Each run's stdout is a JSON array of records, or a stream of JSON objects.
// Any failing run fails the populate phase and nothing is ingested. argv and
// set are recorded with the model instance: keep secrets out of them.
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

// #PluginPlan — RETIRED converge arm (docs/retired-commands.md). Kept so a
// model that still declares `converge:` compiles and the loader can reject it
// with an error naming the field.
#PluginPlan: {
	plugin:          string
	input?:          {...}
	sealed_inputs?:  _
	sealed_outputs?: _
}

// #ValueBinding selects one scalar field from one exact typed resource.
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

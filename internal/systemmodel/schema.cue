package systemmodel

// #SystemModel — a packaging of the IDEA layers / ACUTE phases behind one
// declaration. pudl owns this schema; `pudl run <model>` loads an *instance*
// of it (the run unit) and orchestrates its phases. See
// docs/design/system-models/V1-BUILD-SPEC.md.
//
// V1-narrowed: populate is #PluginObserve | #EweTarget; converge is #PluginPlan
// only (ewe-converge deferred).
#SystemModel: {
	// Catalog classification: a #SystemModel instance is itself a catalog
	// resource (so `pudl schema list` shows the schema and instances can be
	// inventoried). Hidden + concrete, so loading/validating an instance never
	// has to supply it.
	_pudl: {
		schema_type:     "base"
		resource_type:   "system_model"
		identity_fields: ["name"]
		tracked_fields: ["populate", "desired", "converge", "checks", "plugins"]
	}

	name: string

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
	populate: #PluginObserve | #EweTarget

	// RELATE — derived relationships (pudl Datalog rule references).
	relations?: [...string]

	// CHECK — the observe-only flag queries (pudl Datalog).
	checks?: [...#Check]

	// DESIRED — declared desired state (IDEA Definition layer). Present → the
	// model can converge; absent → observe-only.
	desired?: [...{...}]

	// CONVERGE — close drift (ACUTE Transform + Execute). V1: #PluginPlan only.
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
	name:     string & !=""
	command?: [...string] // run this argv directly (e.g. ["bb", "p.bb"] or ["/abs/binary"])
	script?:  string      // local .bb / binary path (relative to the model dir, or abs)
	digest?:  string      // content digest, resolved from the mu plugin cache
	url?:     string      // remote bundle
	sha256?:  string      // required with url
}

// #PluginObserve — reuse a shipped observer plugin (the `host`/`k8s` case). Its
// observe op runs and its output is ingested as the live side. `plugin` names a
// #PluginDef declared in the model's `plugins:` block.
#PluginObserve: {
	plugin: string
	input: {...}
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
	eweSource: string
	outputs: [...string]
	network?:            bool | *false
	impure?:             bool | *false
	sealed_inputs?:      {[string]: string}
	sealed_input_modes?: {[string]: string}
}

// #PluginPlan — converge via a declarative-apply plugin. pudl routes `desired`
// to the plugin as sources; the plugin reconciles. See V1-BUILD-SPEC §5.5.
#PluginPlan: {
	plugin: string
	input?: {...}
}

// #Check — an observe-only flag: evaluate a Datalog relation, assert empty /
// nonempty, attach a severity.
#Check: {
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

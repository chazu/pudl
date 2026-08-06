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
		schema_type:   "base"
		resource_type: "system_model"
		identity_fields: ["name"]
		tracked_fields: ["populate", "desired", "converge", "checks", "plugins", "depends_on"]
	}

	name: string
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
	populate: #PluginObserve | #EweTarget

	// RELATE — derived relationships (pudl Datalog rule references).
	relations?: [...string]

	// CHECK — the observe-only flag queries (pudl Datalog).
	checks?: [...#Check]

	// DESIRED — declared desired state (IDEA Definition layer). Present → the
	// model can converge; absent → observe-only.
	desired?: [...#DesiredResource]

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
	#SealedInputs
	plugin: string
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
	#SealedInputs
	eweSource: string
	outputs: [...string]
	network?: bool | *false
	impure?:  bool | *false
}

// #PluginPlan — converge via a declarative-apply plugin. pudl routes `desired`
// to the plugin as sources; the plugin reconciles. See V1-BUILD-SPEC §5.5.
//
// HOST CREDENTIALS: mu executes the converge actions with a *hermetic*
// environment (no inherited `HOME`/`KUBECONFIG`), so any host credential the
// plugin needs must be passed explicitly through `input` — pudl carries no
// domain knowledge to inject it. For the k8s plugin this means
// `input.kubeconfig: "/abs/path/to/kubeconfig"`; without it `kubectl` cannot
// find `~/.kube/config` and apply fails with `context "…" does not exist`.
#PluginPlan: {
	#SealedExecution
	plugin: string
	input?: {...}
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

// Sealed inputs belong directly to the populate or converge arm that consumes
// them. V1 sealed outputs belong only to converge: populate must finish before
// the complete exact-plan approval boundary and therefore cannot write.
#SealedInputs: {
	sealed_inputs?: {[string]: #SealedInput}
}

#SealedExecution: {
	#SealedInputs
	sealed_outputs?: {[string]: #SealedOutput}
}

#SealedInput: {
	delivery_mode: "env" | "file"
	({
		ref:     string & !=""
		source?: _|_
	} | {
		ref?: _|_
		source: {
			model:  string & !=""
			output: string & !=""
		}
	})
}

#SealedOutput: {
	ref:        string & !=""
	store_mode: "create" | "overwrite" | "create_if_absent"
}

// #DesiredResource — one declared resource. Open: the fields a resource carries
// belong to its own schema, not to #SystemModel, so anything is allowed through.
// What is typed here is the one field PUDL itself interprets.
#DesiredResource: {
	// DEPENDS_ON — other resources in this same model's `desired` list that must
	// be reconciled before this one. Consumed by `pudl run --converge --only`:
	// naming a resource pulls in its declared dependencies transitively, so a
	// scoped converge cannot apply something whose prerequisites it excluded.
	//
	// RESOLUTION RULE. Each entry is a *selector*, resolved against this model's
	// desired list by the same rules as `--only` itself, and it must resolve to
	// exactly one resource:
	//
	//   - an IDENTITY key — `name`, `id`, `path`, `target`, or `metadata.name` —
	//     names exactly one resource. This is the form to use.
	//   - a TYPE key — `_schema`, `schema`, `definition`, or `kind` — names every
	//     resource of that type, so it is an ERROR as a dependency: an edge points
	//     at one resource, not a set. (`--only` accepts type keys; a dependency
	//     does not.)
	//   - the short name after a schema's `#` is accepted for either.
	//
	// A selector matching several resources by identity, or matching one by
	// identity and others by type, is an error rather than a silent pick — see
	// Defect 2 in docs/architecture-improvement-report.md, where flattening the
	// two key classes into one namespace pulled resources into converge scope
	// that the operator never named.
	//
	// Note this is NOT the compound catalog identity `<schema>|<field>/<field>`
	// that `recordIdentity` builds for inventory matching. That key is
	// schema-relative and only exists once a record has been observed; a desired
	// resource's dependency is declared before anything is observed, so it is
	// stated in the model's own terms.
	//
	// Resource dependencies are deliberately NOT emitted as facts. `--only`
	// resolves them at plan time from the loaded model, and deriving converge
	// scope from catalog facts would make it depend on mutable state.
	// Model-level `depends_on:` (which IS emitted, as `model_depends_on`) is a
	// different relation with a different namespace regime — see D4.
	depends_on?: [...string]

	...
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

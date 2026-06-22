package systemmodel

// #SystemModel — a packaging of the IDEA layers / ACUTE phases behind one
// declaration. pudl owns this schema; `pudl run <model>` loads an *instance*
// of it (the run unit) and orchestrates its phases. See
// docs/design/system-models/V1-BUILD-SPEC.md.
//
// V1-narrowed: populate is #PluginObserve | #EweTarget; converge is #PluginPlan
// only (ewe-converge deferred).
#SystemModel: {
	name: string

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

	// VAULT — sealed inputs the populator/converger needs (NAME → secret ref).
	vault?: {[string]: string}
}

// #PluginObserve — reuse a shipped observer plugin (the `host`/`k8s` case). Its
// observe op runs and its output is ingested as the live side.
#PluginObserve: {
	plugin: string
	input: {...}
}

// #EweTarget — a custom ewe fetch program (the GitLab case). See
// ewe-populate-spec.md. (Schema present for the union; ewe internals unbuilt.)
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

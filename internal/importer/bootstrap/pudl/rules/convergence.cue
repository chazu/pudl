package rules

// Cross-model dependency reasoning over the model_depends_on EDB.
//
// model_depends_on(from, to) facts are emitted by `pudl run` from each model's
// declared `depends_on:` (see docs/cross-model-dependencies.md). These rules are
// plain top-level fields (NOT #-prefixed definitions) so the rule loader picks
// them up — ParseRules skips definitions.
//
// Arg-key contract (load-bearing — facts, rule body atoms, and `pudl query`
// constraints must all agree):
//   model_depends_on / depends_transitive : from, to
//   impacted_by                           : changed, impacted
//   cyclic                                : model

// depends_transitive — transitive closure of model_depends_on.
// Base case reads the EDB directly (no redundant `depends` alias).
depends_transitive_base: {
	head: {rel: "depends_transitive", args: {from: "$A", to: "$B"}}
	body: [{rel: "model_depends_on", args: {from: "$A", to: "$B"}}]
}

// Recursive step — evaluated by the semi-naive fixpoint engine. The shared $B
// is the equi-join between a direct edge and the closure so far.
depends_transitive_rec: {
	head: {rel: "depends_transitive", args: {from: "$A", to: "$C"}}
	body: [
		{rel: "model_depends_on", args: {from: "$A", to: "$B"}},
		{rel: "depends_transitive", args: {from: "$B", to: "$C"}},
	]
}

// impacted_by — reverse direction / blast radius: when `changed` changes, every
// `impacted` model transitively depends on it. Intention-revealing keys so the
// query direction (`pudl query impacted_by changed=network`) is unambiguous.
impacted_by: {
	head: {rel: "impacted_by", args: {changed: "$X", impacted: "$A"}}
	body: [{rel: "depends_transitive", args: {from: "$A", to: "$X"}}]
}

// cyclic — a model that transitively depends on itself. A dependency cycle has
// no valid run order; this surfaces it for the user to fix. The repeated $A in
// one body atom compiles to a self-equality (from == to).
cyclic: {
	head: {rel: "cyclic", args: {model: "$A"}}
	body: [{rel: "depends_transitive", args: {from: "$A", to: "$A"}}]
}

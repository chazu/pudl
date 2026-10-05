# CUE functions assessment

**Date:** 2026-10-05

**Status:** Decided — do not adopt yet. Revisit when the triggers below are met.

**Scope:** Whether the experimental `functions` feature in CUE v0.18.0-alpha.2
(and the other v0.17/v0.18 language changes) can simplify pudl's implementation
or functionality.

## Summary

CUE v0.18.0-alpha.2 adds first-class functions behind `@experiment(functions)`.
For pudl they are a good fit in exactly one place — schema identity — and a
poor fit everywhere else. They cannot be adopted on their own: the feature
requires language version v0.18.0, and that version's now-stable strict
embedding breaks pudl's shipped system-model schema. Function values are also
opaque to the Go API, which rules them out wherever Go must introspect what
CUE declares.

Recommendation:

1. Stay on CUE v0.16 (or v0.17.x) until v0.18.0 is released stable.
2. Then migrate to language v0.18.0 as a standalone change (strict embedding).
3. Then prototype identity-as-a-function behind the experiment and measure the
   per-record import cost before committing to it.

pudl currently pins `cuelang.org/go v0.16.0`; workspaces are written at
`language: version: "v0.16.0"`.

## What the release provides

**Functions (experimental, per file).** Proposal: cue-lang/cue discussion
#4484.

```cue
@experiment(functions)

add:   func(a: int, b: int = 10) -> int: a + b
x:     add(1)            // 11
y:     add(1, b: 2)      // positional and labeled arguments
f:     add(1, ...)       // partial application: a function of b
T:     func(a: int, ...) -> number   // bodyless literal = function type
```

- Parameter forms: named (`a: int`), required name-only (`a!:`), optional
  name-only (`a?:`), positional-only with alias (`_~x: int`), anonymous.
- Defaults are declared with `=` and apply only when an argument is omitted.
- A function type unified with a function value tightens it; constraints are
  checked per call.
- Recursion (direct or mutual) is a structural-cycle error. CUE stays
  non-Turing-complete.
- A function value unifies only with itself and with compatible function types.
- Calls are memoized per call site.
- Explicitly out of scope: foreign function interfaces (calling host-language
  code), purity and side effects, variadics, overloading, and dependent
  parameter types. FFI is deferred to a follow-up proposal.

**Stable from language v0.18.0.**

- *Strict embedding* (formerly `explicitopen`): a struct that embeds a closed
  value is closed to that value's fields. `#A...` embeds without closing.
  `cue fix --exp=explicitopen` migrates.
- *Postfix aliases and `self`* (formerly `aliasv2`): `x~(X):`,
  `[string]~(K,_):`; the old prefix alias syntax is rejected.
  `cue fix --exp=aliasv2` migrates.

**Still experimental.** `try` with `?`-marked references for possibly-absent
fields (`try { r.metadata?.namespace? } else { "" }`), and short-circuit
`&&`/`||`.

**Go API.** `cue.Unify` for unifying many values at once, `ast.Clone`, and
`ast.DocComments`/`ast.ResolveComments`. There is no API for calling,
constructing, or reflecting on function values.

## Verified constraints

Each point below was checked by a probe program against
`cuelang.org/go@v0.18.0-alpha.2` (see [Appendix](#appendix-probes)).

1. **Language version gate.** A module at `v0.16.0` using the experiment
   fails to load: `cannot set experiment "functions" before version v0.18.0`.
   Every pudl workspace would need its module version raised.
2. **The version bump breaks shipped CUE.** Under strict embedding, the
   pattern

   ```cue
   #PluginObserve: {
   	#SealedInputs
   	plugin: string
   }
   ```

   fails with `#PluginObserve.plugin: field not allowed`. The same pattern is
   in `internal/systemmodel/schema.cue:92,114,131,156`, and may be in user
   schemas. pudl's shipped CUE uses no prefix aliases, so the alias change
   costs nothing.
3. **Functions are opaque to Go.**
   - `Value.Kind()` reports `func`.
   - `Fields()` fails: `cannot use value func(...) ... (type func) as struct`.
   - A function field cannot be serialized: `MarshalJSON` errors with
     `cannot convert value "func(...)" of type *adt.FuncValue to JSON`, and
     `Decode` of the enclosing struct into a `map[string]any` panics in the
     alpha's decoder (`reflect.Value.Set on zero Value`). Any value pudl
     decodes or stores must keep functions in hidden or definition fields.
   - There is no `Value.Call`. Go can only invoke a CUE function the way it
     already applies struct "functions": `FillPath` an input field, then
     `LookupPath` a field whose expression is the call.
   - The only way to read a signature is `Syntax(cue.Raw())`, which yields an
     `*ast.Func`. Parameter attributes such as `@pudl(binding=plain)` survive
     there, but are dropped from the evaluated value (the proposal notes
     parameter attributes are "parsed and formatted but not yet read by the
     compiler").
4. **No Go-implemented functions.** pudl cannot expose Go functions (identity
   hashing, catalog lookups) to CUE until the FFI proposal lands.
5. **No recursion.** Datalog evaluation, which is recursive, cannot move into
   CUE.
6. **Per-file opt-in.** A file without `@experiment(functions)` rejects
   `func` syntax (`function syntax requires @experiment(functions)`), so
   generated and user-authored files would have to carry the attribute.

## Fit by area

### Schema identity — good fit, deferred

**Today.** Each schema's hidden `_pudl` block lists `identity_fields`
(dotted paths). Go resolves them over the record in
`internal/identity/extract.go:13` (`ExtractFieldValues`). A family invariant —
a child schema's `identity_fields` must equal its base's — is checked
separately by `pudl doctor` (`internal/doctor/checks.go:512`). An optional
nested identity field, such as the k8s `metadata.namespace` on cluster-scoped
objects, makes extraction fail, and the importer falls back to the content
hash.

**With functions.**

```cue
@experiment(functions)
@experiment(try)

#Resource: {
	_pudl: identity: func(r: {...}) -> [...string]: [
		r.kind,
		r.metadata.name,
		try { r.metadata?.namespace? } else { "" },
	]
	...
}
```

- Identity is declared next to the schema and computed by CUE; the Go
  dotted-path resolver goes away.
- `try` handles optional fields. Verified: a `Namespace` with no namespace
  yields `["Namespace", "default", ""]`; a namespaced `Pod` yields
  `["Pod", "web", "prod"]`.
- Children inherit the function through `#Base & {...}`. A child that
  declares a different function fails to unify (verified: `conflicting
  values func(...) and func(...)`), so the doctor family check becomes a CUE
  error.

**Costs and caveats.**

- Import would run a CUE call for every record.
- Inference scoring (`internal/inference/heuristics.go:102`) reads
  `IdentityFields` to rank candidates; a function hides that list, so both
  would have to coexist.
- Most of this is already achievable without functions, as a hidden derived
  field evaluated during the validation unify pudl already performs.
  Functions make it cleaner, not newly possible.

### System model templates — not viable until signatures are introspectable

`ModelTemplate.Elaborate` (`internal/systemmodel/template.go`) already treats a
model as a function: Go fills `inputs`, unifies, validates, and decodes. A
real function (`func(value: string @pudl(binding=plain)) -> sm.#SystemModel`)
would add arity and required-argument checking and remove some Go-side
validation, such as the inputs/bindings key-set check.

But Go must enumerate a model's inputs and their binding classes for
summaries, binding resolution, and `collectBindingClasses`
(`internal/systemmodel/template_decode.go:103`). Function signatures can only
be read from raw syntax, and parameter attributes are not evaluated.
Revisit when CUE exposes signature reflection.

### Datalog rules — no fit

Rules are data that pudl's Go engine interprets; functions are opaque and
cannot be serialized. Recursion is forbidden, so evaluation cannot move into CUE.
Typed helpers for aggregates and comparisons — replacing the regex-parsed
`"count($S)"` and `">0.25"` strings (`internal/datalog/types.go:103`) — would
only produce the same data. The rule loader also compiles each file
standalone, so a shared helper library would not resolve.

### Inference, validation chain, ingest routing — no fit

These are unification plus Go control flow (candidate ordering, fallback,
mapping tables). Functions offer nothing here.

### Other v0.17/v0.18 features

- `cue.Unify` (multi-value): no use. pudl has four single `Value.Unify` call
  sites and no unify chains; its "chains" are alternatives tried against the
  same data.
- `try`: useful only together with identity-as-CUE.
- Comprehension ordering and numeric `==` fixes: no known pudl dependency.

## Migration prerequisites

Before any functions work, as a standalone change once v0.18.0 is stable:

1. Bump `cuelang.org/go`.
2. Run `cue fix --exp=explicitopen` over the bootstrap tree
   (`internal/importer/bootstrap/`) and `internal/systemmodel/schema.cue`.
3. Raise the version pudl writes in `internal/init/init.go:268` and
   `internal/repo/init.go:176`, and in test fixtures (several still use
   `v0.14.0`).
4. Check generated CUE text for embedding patterns: `internal/schemagen`,
   `cmd/model_new.go`, and `cmd/model_populator_new.go`.
5. Provide a migration path for existing user workspaces, whose schemas may
   embed definitions alongside sibling fields.

## Revisit triggers

- CUE v0.18.0 released stable (unblocks the migration).
- The `functions` experiment graduates to a language version.
- A Go API for reflecting on or calling function values (unblocks model
  templates).
- The FFI follow-up proposal (would allow Go-backed functions such as catalog
  lookups).

## Related fixes

The survey behind this assessment found three unrelated defects. They are
fixed in `8892b68`, `a13adb5`, and `08b39f1`; see
`implog/2026_10_05_cue_usage_fixes.md`.

## Appendix: probes

Run against `cuelang.org/go@v0.18.0-alpha.2` with `CompileString` (no module)
unless noted.

| Probe | Result |
|---|---|
| Module at `v0.16.0` using `@experiment(functions)` | `cannot set experiment "functions" before version v0.18.0` |
| Same module at `v0.18.0` | Loads, `f(1)` evaluates |
| `#X: { #SealedInputs; plugin: string }` at `v0.18.0` | `#X.plugin: field not allowed` |
| `Fields()` on a function value | `cannot use value func(...) (type func) as struct` |
| `Syntax(cue.Raw())` on a function value | `*ast.Func`, attributes intact |
| `MarshalJSON` of a struct with a regular function field | Error: `cannot convert value ... of type *adt.FuncValue to JSON` |
| `Decode` of that struct into `map[string]any` | Panic in `cue.(*decoder).decode` |
| Call from Go via `FillPath("in", data)` + `LookupPath("out")` where `out: f(in)` | Works |
| Child redefining an inherited `_pudl.identity` function | `conflicting values func(...) and func(...)` |
| `try { r.metadata?.namespace? } else { "" }` | `""` when absent, value when present |

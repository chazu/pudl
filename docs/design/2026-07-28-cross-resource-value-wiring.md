# Design: cross-resource value wiring through CUE elaboration

**Date:** 2026-07-28  
**Scope:** PUDL, mu, and the typed catalog  
**Status:** Proposed — adversarial review complete; secret path deferred  
**Predecessor:** `docs/design/2026-07-27-swamp-parity-roadmap.md` §13

## Summary

Add bounded value flow between `#SystemModel` instances by treating a model as
a CUE template with declared input slots. PUDL resolves each slot from one
typed catalog resource, injects the concrete value into the authored CUE value,
and only then decodes and runs the model.

The shape is:

```text
producer model
    │ observe typed resource
    ▼
catalog snapshot + provenance
    │ resolve binding
    ▼
concrete CUE inputs
    │ unify with model template
    ▼
validated concrete #SystemModel
    │ render desired state
    ▼
mu
```

CUE remains the lingua franca for types, model structure, and value
propagation. PUDL remains responsible for catalog selection, observation reuse,
cardinality, provenance, and run orchestration. mu receives concrete model
inputs and does not query the catalog.

## The motivating case

Model `app` needs the subnet ID discovered by model `network`:

```cue
package models

App: sm.#SystemModel & {
	name: "app"

	inputs: {
		subnet_id: string
	}

	bindings: {
		subnet_id: {
			source: {
				model:  "network"
				schema: "pudl/aws.#Subnet"
				identity: {SubnetId: "subnet-private"}
			}
			path: "SubnetId"
		}
	}

	desired: [{
		_schema: "pudl/example.#Application"
		name:    "api"
		network: {subnet_id: inputs.subnet_id}
	}]
}
```

This is illustrative syntax for the proposed `inputs` and `bindings` fields;
neither field exists in `#SystemModel` yet. The important property is that the
desired resource refers to a CUE value (`inputs.subnet_id`), not to a PUDL
function, string interpolation expression, SQL fragment, or mu-specific
placeholder.

After resolution, PUDL unifies the template with:

```cue
{inputs: {subnet_id: "subnet-0123456789"}}
```

The resulting desired resource is concrete before it is decoded, structurally
validated, and passed to mu.

## Domain terms

- **Producer model:** a `#SystemModel` whose populate phase records the typed
  resource that supplies a value.
- **Consumer model:** a `#SystemModel` whose template declares an input slot.
- **Typed resource:** one catalog item assigned to a loaded PUDL schema, with
  schema-relative identity and observation provenance.
- **Observation snapshot:** the durable record of what a model observed during a
  run. Existing `observe_snapshots.model`, `snapshot_id`, `run_id`, and
  collection memberships provide the provenance boundary.
- **Input slot:** a named CUE constraint such as `inputs.subnet_id: string`.
- **Binding:** the declaration that tells PUDL which typed resource and field
  supply an input slot.
- **Elaboration:** resolving bindings and unifying concrete inputs into the
  authored CUE value before `DecodeValue` produces the Go run unit.

The words “dependency” and “reference” must stay qualified. A model dependency
is an execution/data edge; a resource reference is a selector for one catalog
item. Existing `depends_on` remains ordering and impact metadata, not a value
expression.

## Proposed CUE contract

The base model schema would gain open, optional authoring fields similar to:

```cue
#SystemModel: {
	// Existing fields remain unchanged.
	inputs?:   {...}
	bindings?: {[string]: #ValueBinding}
}

#ValueBinding: {
	source: #ResourceRef
	path:   string
}

#ResourceRef: {
	model:    string & != ""
	schema:   string & != ""
	identity: {[string]: _}
}

```

The value type is deliberately expressed by the input slot, not duplicated in
the binding. `inputs.subnet_id: string` is what CUE checks after elaboration.
The binding supplies a value; it does not become a second type system.

### Scalar path grammar

`path` is an RFC 6901 JSON Pointer into the concrete catalog resource, for
example `/SubnetId` or `/metadata/name`. It is not a CUE path or dot path.
Segments use the standard `~0` and `~1` escapes and match object field names
exactly and case-sensitively.

The first slice traverses concrete object fields only. The empty root pointer,
arrays at any position, numeric list indexing, wildcards, filters, and relative
paths are rejected. The final value must be a concrete JSON string, number,
boolean, or explicit `null`; objects and arrays are not scalar leaves. Missing
and `null` are distinct: a real `null` is injected and succeeds only when the
input's CUE constraint permits it.

CUE aliases, definitions, optional constraints, and hidden fields are not
addressable because resolution operates on the concrete exported resource
rather than the schema's authoring structure. Schema-level sensitivity policy
is a separate eligibility check and does not change path syntax.

The first implementation should require these invariants:

1. Every binding name names an input slot.
2. Every declared input slot has exactly one same-named binding.
3. `source.schema` normalizes to a loaded canonical PUDL schema.
4. `source.identity` selects exactly one resource in the permitted snapshot.
5. `path` resolves to one concrete leaf value.
6. The injected value unifies with the input slot and the final model.

`inputs` is a flat struct of visible top-level scalar constraints in the first
slice. Optional and nested slots are rejected, binding names are exact field
labels rather than paths, and extra or unbound names are preflight errors.
Defaults do not make a declared input optional; local constants belong outside
the external input contract. Every declared input must be bound even when it is
not currently referenced by `desired` or another phase.

Because `bindings` is a keyed CUE struct, compatible repeated declarations
unify into one value and conflicting declarations are ordinary CUE errors.
PUDL does not add AST-level duplicate detection on top of CUE.

Static input/binding shape and completeness validation occurs during run-set
preflight before any member executes. Bound values are resolved and unified
after producers succeed, and the final model must pass concrete validation
before the consumer run or mu plan begins.

Bindings are carried by a separate template object outside the decoded Go
`SystemModel`. They are authoring metadata, not desired state.

## Elaboration boundary

The current resolver discovers a model, calls `systemmodel.DecodeValue`, and
returns `*systemmodel.SystemModel`. Wiring cannot be added after that point:
doing so would inject ordinary Go values after CUE has already validated the
model, defeating CUE's type-checking role.

The resolver should instead expose a two-stage path:

```text
load authored CUE value
    → validate as an incomplete template
    → inspect bindings and build model dependency edges
    → run/locate producer snapshots
    → resolve concrete input values
    → cue.Value.Unify(concrete inputs)
    → validate concrete value
    → DecodeValue
    → existing populate/drift/check/converge/report path
```

`cue.Validate(cue.Concrete(false))` is appropriate for the authored template;
the final value must pass concrete validation before the normal run path begins.
The existing `decodeDesired` behavior remains valuable because hidden
`_schema` routing fields must survive the final decode.

### Model template seam

Discovery returns a `systemmodel.ModelTemplate` rather than prematurely
decoding every definition into a runtime model. The template contains the
loaded in-memory `cue.Value`—the evaluated definition with its CUE context,
imports, constraints, and references—plus its concrete model name, canonical
definition name, package/load directory, owning PUDL root, definition origin,
and decoded binding declarations.

Discovery validates this template value with `cue.Concrete(false)` and decodes
only the concrete identity and binding metadata needed for preflight. A valid
model is not skipped merely because an input reference leaves `desired` or
another runtime field incomplete.

After producers succeed, `ModelTemplate.Elaborate` builds the concrete input
object in the template value's existing CUE context, immutably unifies it,
performs final concrete validation, and calls `DecodeValue` to produce the
ordinary `SystemModel`. Each elaboration returns a fresh value; resolved inputs
are not cached across runs. Existing models without inputs take the same path
with an empty input object.

The template value is retained only in memory for discovery and elaboration. It
is not source text, is not persisted to the catalog, and is never mutated by
input injection.

`inputs` and `bindings` remain template-only. They are absent from
`SystemModel`, so the existing explicit Go runtime projection structurally
excludes them from model-instance catalog serialization and generated mu input.
Concrete non-secret values may still appear naturally in `desired`, plugin
configuration, checks, or another runtime field that referenced an input.
Binding provenance belongs in run reports, and any persisted dependency edge
belongs in the dependency relation rather than model-instance JSON.

Contract coverage must use sentinel authoring metadata to prove that neither
the catalog model-instance record nor the generated mu input contains the
`inputs` or `bindings` namespace.

## Execution boundary

PUDL owns model-level coordination because catalog selection and CUE
elaboration happen between producer and consumer model executions. mu retains
its existing boundary: it executes one elaborated model's internal target and
action graph at a time and does not acquire a catalog client or cross-model
value resolver.

The multi-model surface is a separate command:

```text
pudl run-set <model> <model>...
```

Its library entry point accepts a `RunSetRequest` with an exact, explicitly
named model set. `pudl run <model>` keeps its existing single-model semantics.
The run set never expands itself by discovering upstream or downstream models.
A binding whose producer is outside the set is a preflight error; an authored
`depends_on` target outside the set retains its existing advisory status and
does not expand or block the set.

Within the selected set, binding edges and authored `depends_on` edges both
constrain execution order. Producers execute before consumers, with lexical
model-name ordering for otherwise independent members. Duplicate model names,
self-dependencies, and cycles are rejected before any member model executes.

One run-set ID groups the operation, while every model retains its own run ID,
snapshot ID, and snapshot ownership. A failed model blocks its transitive
consumers, but independent branches continue. The command exits nonzero when
any member failed or was blocked.

The first run-set contract is observe-only and rejects convergence-specific
flags. Each member completes its existing observe-only lifecycle—observation
or populate, drift evaluation when applicable, checks, and reporting—before
its consumers may elaborate. Ordinary drift does not invalidate observed
values, but an execution error or failed fail-severity check blocks consumers.
An ingested snapshot is therefore not exposed early and cannot be consumed
before the producer's terminal result is known.

Multi-model convergence is deferred to a separate design decision covering
group approvals, apply budgets, partial mutation, and final re-observation.
Single-model `pudl run <model> --converge` remains unchanged.

## Source selection and authority

The authoritative producer output is a typed catalog item in a model-owned
observation snapshot. A binding does not read a raw JSON file, the producer's
desired declaration, or a last-written filesystem artifact.

An observation is successful only when its owning model run reaches the
successful terminal state defined by the full observe-only lifecycle above.
The `runs` audit record therefore gains a structured completion status:
`running`, `succeeded`, `failed`, or `blocked`. This status is distinct from
the model verdict: a successfully completed observation may report ordinary
drift and still supply valid observed values.

Snapshot eligibility is derived by joining immutable snapshot provenance to
its owning run, not by storing a second usability flag on the snapshot. The
snapshot must have matching non-empty model and run IDs, represent a real typed
observation, and join to a `succeeded` run. Model-instance registration rows,
differential-drift evidence, and snapshots owned by failed, blocked,
unfinished, or crashed runs are not value sources. Failure to persist terminal
success is a producer failure and blocks its consumers.

Historical run rows without provable structured success remain ineligible;
operators must take a fresh producer observation rather than treating ambiguous
legacy state as successful.

Resolution selects and pins the snapshot before looking for the requested
resource. In a run set, it uses the exact successful snapshot owned by that
producer member. In a standalone run, it chooses the newest eligible snapshot
for the exact producer model and the invocation's current `EffectiveOrigin`
workspace. It never falls back across repository workspaces or from a
repository workspace to `global`.

Snapshot origin and source are system provenance constraints, not
author-facing binding fields. Selection and membership/resource lookup operate
against one consistent catalog read view, and every subsequent lookup uses the
pinned snapshot ID. A concurrently committed observation is therefore eligible
only for a later consumer run.

Within the pinned snapshot, resolution searches item memberships and validates
the candidate against the requested canonical schema. Identity keys are
interpreted using that schema's `_pudl.identity_fields`, not as a global name
lookup. If the newest eligible snapshot does not contain the resource,
resolution reports absence; it does not walk backward to an older snapshot
where the resource still existed.

The source selector therefore has three parts:

```text
(producer model, canonical schema, schema-relative identity)
```

### Review decision 1 — semantic authoring selectors

Accepted during adversarial review: model files use the semantic selector above
so they remain valid across observations. PUDL resolves that selector to the
concrete catalog entry, snapshot, run, and content evidence internally and
records those values in the run report. Authors do not pin a content-addressed
entry ID in CUE.

The field `path` is evaluated only after that one item is selected. A binding
must never silently choose the newest row across all models or all origins. It
may use the latest successful observation for the exact `(model, schema,
identity)` selector under the reuse convention below.

## Observation reuse and run consistency

Observation reuse is an execution convention, not per-binding CUE
configuration. Authors declare what to read; PUDL decides which successful
observation supplies it according to the run context:

- If the producer participates in the current orchestrated run, use its
  successful snapshot from that run.
- If the consumer runs alone, pin the latest successful snapshot for the exact
  producer model in the current workspace, then resolve schema and
  schema-relative identity within that snapshot.
- If no successful snapshot exists, fail before mu planning. A producer that
  fails in the current orchestrated run must not silently fall back to an older
  snapshot.
- Always record whether the value came from the current run or a prior
  snapshot, along with its observed age, in the run report.

A current-run producer that fails, is blocked, fails a fail-severity check, or
produces no eligible snapshot blocks every dependent consumer regardless of
historical data. A standalone consumer has no current-run producer and may use
the newest eligible successful snapshot even when a newer producer attempt
failed, subject to the workspace and age policies above. Its report must
disclose both the reuse and the newer failed attempt. If no eligible snapshot
exists or the pinned snapshot exceeds the age bound, standalone elaboration
fails before constructing a mu plan and never starts the producer implicitly.

`pudl run` and `pudl run-set` accept an optional
`--max-observation-age <duration>` run policy, represented by an optional
`MaxObservationAge` field in the library request. The first release has no
config-file default. The policy is not part of the binding CUE API and does not
reuse the model's loop-cadence `freshness` field.

PUDL evaluates age once when consumer elaboration begins, using the pinned
snapshot's `created_at`, and applies the bound equally to current-run and reused
snapshots. An over-age snapshot fails the binding; it does not make another
snapshot eligible or trigger a producer. Reports record the snapshot time,
evaluation time, observed age, configured bound, and status. If later policy
layers add other bounds, PUDL uses the smallest one so policy may tighten but
never loosen selection. Omitting the policy performs no age rejection.

When models are orchestrated together, binding edges establish the phase order:
producer populate must complete and commit its snapshot before consumer
elaboration. Automatic producer execution can be added later; the first slice
does not hide external work behind a value lookup.

The consumer sees the producer snapshot from that same run when one exists, and
otherwise the scoped latest successful observation by convention.

## Cardinality and failure semantics

Binding resolution is fail-closed. The resolver returns a structured diagnostic,
not a fallback value:

| Condition | Result |
|---|---|
| no matching item | unresolved: source absent |
| more than one matching item | unresolved: source ambiguous |
| producer snapshot failed | unresolved: producer failed |
| snapshot exceeds an explicit run-level age bound | unresolved: source too old |
| schema missing | unresolved: schema unavailable |
| path missing or non-leaf | unresolved: projection invalid |
| value conflicts with CUE input | unresolved: type mismatch |
| all checks pass | concrete input value |

An unresolved binding prevents the consumer's mu plan or apply from being
created. It must not become `null`, an empty string, `pudl/core.#Item`, or an
unconstrained CUE value. Dry-run and machine-readable reports should expose
the same diagnostic without causing external side effects.

## Run reports and facts

The run report should gain a binding section containing, for each input:

- consumer model and input name;
- producer model, snapshot ID, run ID, schema, and resource identity;
- selected path;
- reuse decision, snapshot and evaluation times, observed age, and any
  run-level age bound;
- resolution status;
- a redacted value summary or value hash, not secret material.

The report is the operator-facing explanation for why a consumer did or did not
run. The binding edge may also be emitted as a model dependency fact, but it
must remain bounded by the current model graph and run retention policy. Do not
add a monotonically growing per-phase relation merely to expose wiring.

## Interaction with mu

PUDL elaborates values before rendering the mu project. mu receives the same
concrete desired state it receives today; it does not need a catalog client,
PUDL schema loader, or cross-model query language.

If an input is needed by a mu plugin as operational configuration rather than
desired resource data, PUDL should place the concrete non-secret value in the
existing model/plugin input or config map only after CUE elaboration and
validation. A secret must not take that path as plaintext.

### Deferred secret compatibility with mu

The future secret path must lower into mu's existing sealed-I/O primitives,
not invent a second PUDL secret channel:

- `sealed_inputs: {[name]: ref}` carries references such as
  `"pass:deploy/token"` on a mu target or action. mu resolves the reference at
  execution time through the provider's `resolve_secret` capability (or the
  built-in `env:NAME` scheme), so the resolved value is not part of authored
  CUE, the catalog, the action key, CAS, manifests, or reports.
- `sealed_input_modes: {[name]: "env" | "file"}` controls delivery. `env` is
  the default; `file` writes a per-action `0600` temporary file and exposes its
  path through the named environment variable. `file` is currently unsupported
  for hermetic toolchain-sandbox actions, so a future binding must preserve
  that execution constraint rather than silently selecting file delivery.
- Execute-phase pith uses `secret/get` to read a declared sealed input as a
  tainted `pith.Secret`; `env/get` and `env/get-default` refuse sealed names.
  Taint is redacted in traces and errors and is revealed only at sanctioned
  effectful sinks such as authenticated HTTP or file writes.
- `sealed_outputs: {[name]: ref}` is the existing write path. An action writes
  `$MU_SEALED_OUT_DIR/<name>`; mu captures it and routes it through the
  provider's `store_secret` capability. `sealed_output_modes` selects
  `create`, `overwrite`, or `create_if_absent`, and project-level
  `secrets.writable_refs` gates writes at plan time and write time.
- The Go plugin SDK exposes these provider capabilities directly through
  `ResolveSecret`/`StoreSecret`, or through the `SecretBackend` plus
  `SecretPlugin` adapter. This is the provider contract Phase 2 must reuse.

The exact model-level spelling and lowering of a secret binding remain
deferred. The required invariant is already fixed: a secret binding lowers to
mu sealed-input references and execution metadata, never to a concrete CUE
scalar. A secret produced by an action likewise remains on the sealed-output
side channel; it does not become an ordinary observation record. If a later
feature needs a catalog-visible fact, it must expose only non-secret metadata
or an opaque handle and receive its own design review.

Canonical mu references for this future work are `docs/guide/protocol.md`,
`docs/guide/pith-plugins.md`, `docs/guide/sdk.md`,
`docs/sealed-input-delivery-modes.md`, `docs/secrets-write-policy.md`, and
`docs/design/pith-sealed-io.md` in the mu repository.

## Alternatives considered

### CUE function or `catalog.latest(...)`

Rejected. CUE has no user-defined functions, and the expression would either
require a preprocessing language or an implicit eager injection mechanism. The
design makes eager injection explicit and keeps catalog selection and the
latest-successful reuse convention in PUDL.

### String interpolation such as `${network.subnet_id}`

Rejected. It bypasses CUE's type system, creates escaping and quoting hazards,
and makes unresolved values indistinguishable from malformed authoring text.

### Lazy catalog references during mu execution

Rejected for the first version. It would give mu a second data authority,
complicate reproducibility, and make a run's desired state depend on reads after
the model was validated.

### Arbitrary Datalog query as a binding

Deferred. Datalog remains the right language for relational queries, but an
arbitrary query result requires a result schema, cardinality contract, temporal
scope, and a safe CUE conversion rule. Start with one typed resource and one
schema-relative path; add query bindings only as a separately designed feature.

## Phased implementation

### Phase 0 — contract tests and template seam

- Preserve the authored `cue.Value` through model resolution.
- Add `#ValueBinding`/`#ResourceRef` validation.
- Add an elaboration result type with concrete inputs and diagnostics.
- Prove CUE type propagation with scalar, nested, missing, and conflicting
  values without invoking mu.

### Phase 1 — one-resource catalog resolver

- Resolve exact `(model, schema, identity)` selectors through current snapshots
  and collection memberships.
- Support scalar leaf paths and the conventional current-run/latest-successful
  observation selection, with an optional run-level age bound.
- Record binding evidence in the run report.
- Add a deterministic producer/consumer integration fixture.

### Phase 2 — orchestrated dependency execution

- Derive binding edges and topologically order producer/consumer models.
- Ensure producer failure prevents consumer planning.
- Preserve explicit `depends_on` as ordering-only metadata.
- Add dry-run and JSON diagnostics for unresolved bindings.

### Deferred follow-ons

- whole-record and list-valued projections;
- arbitrary Datalog query bindings;
- automatic producer execution when a standalone consumer is missing data;
- secret-valued bindings lowered through mu `sealed_inputs` and
  `sealed_input_modes`, with any producer-side capture using
  `sealed_outputs`/`store_secret`;
- explicit historical-version pinning or custom observation selection beyond
  the latest-successful convention;

## Acceptance criteria

The design is ready to implement when these cases are specified and tested:

1. A producer observes one typed subnet and a consumer receives its ID through
   CUE unification.
2. A missing producer value blocks the consumer before mu planning.
3. Two matching producer resources produce an ambiguity error.
4. An orchestrated run uses the producer's successful current-run snapshot; a
   standalone consumer uses the latest successful scoped snapshot, and an
   optional run-level age bound rejects older data.
5. A number injected into a `string` input fails CUE validation without a
   partial run.
6. Producer failure appears in the consumer run report as an unresolved source.
7. The generated mu input contains the concrete value and no binding metadata.
8. No catalog lookup, interpolation, or lazy reference occurs inside mu.

## Adversarial review questions

1. **Resolved:** use the producer model + canonical schema + identity selector
   in authoring; record immutable resolution evidence internally.

### Review decision 2 — explicit producer orchestration

Accepted during adversarial review: the first implementation requires explicit
orchestration. PUDL may reuse an existing successful producer snapshot when the
run convention or an explicit run-level policy permits; otherwise it fails
before mu planning. In a
multi-model run, the binding creates a data dependency so the producer
observation completes before consumer elaboration. Producer execution is not
hidden inside consumer resolution.

2. **Resolved:** a standalone consumer does not trigger its producer; producers
   are explicitly included in an orchestrated run, or an acceptable existing
   snapshot must already be present.

### Review decision 3 — scalar leaf projections first

Accepted during adversarial review: the first implementation resolves scalar
leaf paths only. Whole-record and list-valued projections remain follow-ons.
This keeps path validation, cardinality, and CUE type-mismatch behavior small
and deterministic while covering the initial producer-to-consumer cases.

3. **Resolved:** begin with scalar leaf projections; defer whole records and
   lists until the resolver contract has shipped and been exercised.
### Review decision 4 — binding edges imply data dependencies

Accepted during adversarial review: a binding creates a data-dependency edge
automatically. `depends_on` remains available for ordering-only relationships,
so authors do not need to duplicate a dependency already expressed by a
binding.

4. **Resolved:** infer data dependencies from `bindings`; reserve `depends_on`
   for ordering-only metadata.
### Review decision 5 — convention-over-configuration observation reuse

Revised during follow-up design review: freshness is not a per-binding CUE
field. An orchestrated run uses the producer's successful current-run snapshot;
a standalone consumer uses the latest successful snapshot for the exact
selector. Missing data still fails closed, current-run producer failure is not
masked by fallback, and the report records the reuse decision and age. An
optional run-level age bound may reject older observations without adding
configuration to every binding.

5. **Resolved:** use current-run/latest-successful observation conventions;
   keep stricter age bounds at run policy level, not in CUE bindings.
### Review decision 6 — no secret-valued bindings in the first slice

Accepted during adversarial review: the first wiring slice does not support
secret-valued bindings. Future secret support must lower to mu's existing
`sealed_inputs`/`sealed_input_modes` contract, and any producer-side secret
capture must use `sealed_outputs`/`store_secret`. Plaintext must stay out of
CUE, catalog records, and run reports. Reports may retain provenance,
freshness, status, and a redacted or hashed summary.

6. **Resolved:** exclude secret-valued bindings initially; reserve them for a
   separate sealed-input design with redacted reporting.

## Adversarial review status

Complete for this design pass. The six review decisions above are the current
implementation boundaries; changes to them should be treated as design
revisions rather than incidental implementation details.

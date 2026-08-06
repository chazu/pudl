# Design: cross-resource value wiring through CUE elaboration

**Date:** 2026-07-28  
**Scope:** PUDL, mu, and the typed catalog  
**Status:** Accepted and implemented — Phases 0–3 plus the real sealed run-set
release matrix are complete across PUDL and mu
**Predecessor:** `docs/design/2026-07-27-swamp-parity-roadmap.md` §13

## Summary

Add bounded value flow between `#SystemModel` instances with two deliberately
separate channels. Plain bindings resolve an eligible field from one typed
catalog resource, inject the concrete value into the authored CUE template,
and only then decode and run the model. Sealed bindings carry only a secret
provider reference and lower to mu's existing sealed-input/output machinery;
the secret value never becomes a CUE value or catalog field.

The shape is:

```text
producer model
    ├─ observe eligible plain field ─> pinned catalog snapshot
    │                                  │ resolve + unify
    │                                  ▼
    │                            concrete CUE input
    │
    └─ declare sealed output ────────> provider reference only
                                       │ lower without resolving
                                       ▼
consumer model ───────────────────> mu sealed_inputs / sealed_outputs
```

CUE remains the lingua franca for types, model structure, annotations, and
plain-value propagation. PUDL remains responsible for catalog selection,
observation reuse, cardinality, provenance, sealed-reference wiring, and run
orchestration. mu receives concrete plain model inputs plus sealed references;
it does not query the catalog. Only mu resolves or stores secret values.

## The motivating case

Model `app` needs the subnet ID discovered by model `network`:

```cue
package models

App: sm.#SystemModel & {
	name: "app"

	inputs: {
		subnet_id: string @pudl(binding=plain)
	}

	bindings: {
		subnet_id: {
			source: {
				model:  "network"
				schema: "pudl/aws.#Subnet"
				identity: {SubnetId: "subnet-private"}
			}
			path: "/SubnetId"
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
- **Plain input slot:** a named CUE constraint such as
  `inputs.subnet_id: string`, explicitly annotated as eligible for plain
  binding.
- **Plain binding:** a declaration that tells PUDL which typed resource and
  eligible field supply a concrete CUE input.
- **Sealed output declaration:** a converge-owned name plus the provider
  destination and store mode. It carries metadata, never the secret value.
- **Sealed input declaration:** a phase-local mu input name, delivery mode, and
  exactly one source: either a direct provider reference or a producer model's
  named sealed output.
- **Sealed binding:** the `source` arm of a sealed input declaration. It copies
  the producer output's provider reference into mu `sealed_inputs`; it does not
  bind a secret-valued CUE field.
- **Elaboration:** resolving bindings and unifying concrete inputs into the
  authored CUE value before `DecodeValue` produces the Go run unit.

The words “dependency” and “reference” must stay qualified. A model dependency
is an execution/data edge; a resource reference is a selector for one catalog
item. Existing `depends_on` remains ordering and impact metadata, not a value
expression.

The canonical dependency direction is consumer to producer, matching the
existing `model_depends_on(from, to)` contract: `from` is the model that needs
state and `to` is its prerequisite. Plain bindings and cross-model sealed
sources use the same meaning. A direct sealed `ref` creates no model edge.
Execution planning reverses canonical edges to obtain producer-before-consumer
order; diagrams may show that scheduling direction but do not redefine the
stored relation.

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
		ref:     string & != ""
		source?: _|_
	} | {
		ref?: _|_
		source: {
			model:  string & != ""
			output: string & != ""
		}
	})
}

#SealedOutput: {
	ref:        string & != ""
	store_mode: "create" | "overwrite" | "create_if_absent"
}

// populate arms embed #SealedInputs. Only converge embeds #SealedExecution;
// desired values and Datalog checks embed neither.
```

The value type is deliberately expressed by the input slot, not duplicated in
the binding. `inputs.subnet_id: string @pudl(binding=plain)` is what authorizes
and what CUE checks after elaboration.
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
rather than the schema's authoring structure. Schema-level binding
classification is a separate eligibility check and does not change path
syntax. A plain resource projection is allowed only when the selected schema
field carries `@pudl(binding=plain)`. Unannotated fields fail closed. A
A phase-owned input or converge-owned output declaration marked
`@pudl(binding=sealed)` may participate only in the sealed-reference channel
and is never a secret-valued CUE field or catalog value.

The first implementation should require these invariants:

1. Every binding name names an input slot.
2. Every declared input slot has exactly one same-named binding.
3. `source.schema` normalizes to a loaded canonical PUDL schema.
4. `source.identity` selects exactly one resource in the permitted snapshot.
5. `path` resolves to one concrete leaf value.
6. The injected value unifies with the input slot and the final model.
7. The source schema field and consumer slot both permit plain binding.
8. A sealed input declares exactly one of `ref` or `source`, never both.
9. A sealed `source` selects one model-wide unique producer output name and
   transports only its provider reference; it cannot become a plain input.

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

Sealed input declarations live on the `populate` or `converge` execution arm
that consumes them. Sealed outputs are converge-only in v1. Populate must
finish before PUDL can construct the complete exact mutation plan, so allowing
the same populate action to write would place an external mutation before its
approval. The map key is already the mu-visible input/output name; there is no
separate generic port, `as` alias, or attachment layer.

Ownership remains asymmetric. A producer output owns its provider reference and
`store_mode` because it performs and accounts for the external write. A
consumer input owns its local name and `delivery_mode`. A source-bound input
names only the producer model and output and cannot repeat, replace, or override
the provider reference. A direct-ref input owns its reference because no model
produces it. The current workspace/run-set policy owns
`secrets.writable_refs`, which PUDL forwards unchanged into every generated mu
project that may write.

Preflight validates the exactly-one-of input union, resolves every sealed source
to one annotated producer output, validates provider-ref syntax and scheme
capability, rejects duplicate output names, validates delivery/store modes, and
applies writable-ref policy. A consumer cannot downgrade a sealed declaration
into a plain binding or bind a plain field as sealed.

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
snapshot ID, and snapshot ownership. In an observe-only run-set, a failed model
blocks its transitive consumers while independent branches continue. In a
mutating run-set, the first failure globally stops new mutations: transitive
consumers are `blocked`, independent members that have not started mutation are
`cancelled`, and no `--continue-on-error` escape hatch exists in v1. The command
exits nonzero when any member failed, was blocked, or was cancelled.

`pudl run-set` without `--converge` is observe-only. Each member completes its
existing observe-only lifecycle—observation or populate, drift evaluation when
applicable, checks, and reporting—before its consumers may elaborate. Ordinary
drift does not invalidate observed values, but an execution error or failed
fail-severity check blocks consumers. An ingested snapshot is therefore not
exposed early and cannot be consumed before the producer's terminal result is
known.

`pudl run-set ... --converge` enables a mutating run-set. PUDL completes all
read-only preflight, observation, elaboration, policy validation, and mu
planning before the first mutation. A converging run-set whose model declares
any sealed output is always approval-gated even when the operator omits
`--require-approval`.
Ordinary run-sets without sealed outputs retain the explicit flag's existing
policy.

The approval is bound to an exact canonical plan digest, not merely the model
names and command options. The digest commits to the selected models, graph
edges, pinned snapshots, resolved plain bindings, desired/config projections,
plugin and action identities, sealed provider references, input/output modes,
store modes, writable-ref policy, and convergence options. It never contains a
resolved secret value. The approval display shows complete write destinations
and modes so the operator can review the side effects; the durable approval
record retains the digest plus redacted reference fingerprints.

On approval or resume, PUDL rebuilds and revalidates the complete plan before
execution. Any difference invalidates the approval and performs no mutation;
the replacement plan requires a new approval. A denied request performs no
mutation. A forbidden sealed-output destination fails policy validation before
an approval request is created.

Approval does not claim transactional rollback across external systems. Once
mu successfully calls `store_secret`, a later failure leaves that write in
place and the run report must identify the completed mutation and blocked or
failed remainder. After any mutating member fails, PUDL launches no further
mutations, including on independent branches. It may perform read-only
re-observation, verification, and reporting for actions already attempted.
Existing per-model convergence behavior remains unchanged outside `run-set`.

### Transaction and concurrency boundary

A run-set never holds one SQLite transaction across mu planning, execution, or
another external call. It reserves the run-set ID, member run IDs, and expected
snapshot IDs atomically, then uses a short catalog read transaction to select
and pin every reused snapshot and copy all required binding evidence into an
immutable plan object. The read closes before any subprocess runs.

A current-run producer commits its snapshot, items, memberships, and successful
completion as one short step transaction; consumers address that exact snapshot
ID. The canonical plan digest and pending approval commit together. Approval or
resume reloads model, plugin, and policy fingerprints, reconstructs the plan
from pinned evidence, and compares the digest before mutation. Each completed
execution step and mutation receipt commits in its own short transaction.

Concurrent catalog writes cannot redirect a pinned binding to a newer snapshot.
Pending approvals protect all referenced snapshots from retention pruning. If
required evidence is missing or any plan input changes, approval becomes stale
instead of selecting replacement data. A crash after a possible external
mutation but before its receipt commits yields `needs-verification`/`unknown`;
the mutating run-set does not retry or resume automatically.

## Source selection and authority

The authoritative producer output is a typed catalog item in a model-owned
observation snapshot. A binding does not read a raw JSON file, the producer's
desired declaration, or a last-written filesystem artifact.

An observation is successful only when its owning model run reaches the
successful terminal state defined by the full observe-only lifecycle above.
The `runs` audit record therefore gains a structured completion status:
`running`, `succeeded`, `failed`, `blocked`, or `cancelled`. This status is
distinct from
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

Binding-derived model edges are persisted in the existing
`model_depends_on(from, to)` relation under fact source
`binding:<consumer>`. Reconciliation aggregates all valid plain bindings and
cross-model sealed sources into one wanted producer set, then atomically diffs
that entire set after successful template validation. Removing the last
binding to a producer bitemporally invalidates the binding-sourced fact. An
invalid template never partially rewrites the graph.

The existing `model:<consumer>` declared source, `derived:<consumer>` heuristic
source, and new `binding:<consumer>` authoritative source reconcile
independently. Coincident edges remain independently valid but queries and
`pudl model deps` coalesce the `(from, to)` pair and report combined provenance,
such as `[declared, binding]`. Direct sealed provider refs create no model
edge. Per-input resolution, snapshot, and run evidence belongs in the run
report rather than per-run dependency facts.

Reports are a versioned, two-level public contract. A `RunSetReport`, keyed by
`run_set_id`, records `report_version`, mode, terminal status, canonical plan
digest, approval identity/status, dependency edges, ordered members, and each
member's model/run ID plus `succeeded`/`failed`/`blocked`/`cancelled` result.
Each existing per-model `RunReport`, keyed by `run_id`, gains `report_version`,
`run_set_id`, completion status, binding evidence, and mutation receipts. The
catalog stores both as explicit typed JSON documents rather than unversioned
maps; a dedicated run-set record owns orchestration-level retrieval.

Each plain binding report records the consumer model/run/input, producer
model/run, pinned snapshot, workspace, schema, identity, JSON Pointer,
current-run/reused selection, observation/evaluation times, age and bound,
resolution status, exact JSON scalar/type, and scalar digest. The plain
annotation explicitly authorizes that catalog value to flow and the exact value
keeps the durable report reproducible after snapshot retention.

Each sealed binding report records the consumer model/run/phase/input,
delivery mode and claiming action IDs, direct-ref versus producer-output
source, producer model/run/phase/output/store mode and producing action when
applicable, provider scheme, reference fingerprint, matched writable-policy
rule fingerprint, and lifecycle status (`planned`, `stored`, `resolved`, `delivered`, or
`failed`). It stores neither the secret value nor a secret-value hash; hashing a
low-entropy secret can itself disclose it. Provider paths remain limited to the
live approval display and mu's operational configuration/provider calls.

Every completed external mutation has a typed receipt in the responsible
member report, so successful writes remain auditable if a later member fails.
Errors are structured and redacted. The report is the operator-facing
explanation for why each consumer did or did not run; dependency facts stay
bounded to the current graph rather than growing once per run or phase.

## Interaction with mu

PUDL elaborates values before rendering the mu project. mu receives the same
concrete desired state it receives today; it does not need a catalog client,
PUDL schema loader, or cross-model query language.

If an input is needed by a mu plugin as operational configuration rather than
desired resource data, PUDL should place the concrete non-secret value in the
existing model/plugin input or config map only after CUE elaboration and
validation. A secret must not take that path as plaintext.

### V1 sealed compatibility with mu

The v1 secret path lowers into mu's existing sealed-I/O primitives. PUDL does
not invent a second secret channel:

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
  `SecretPlugin` adapter. This is the provider contract v1 reuses.

PUDL-generated mu targets require strict explicit sealed routing. A phase-level
declaration makes a sealed name available to its plugin plan, but does not grant
it to every emitted action. Each action must explicitly claim every sealed
input it consumes; an input may reach several actions only through several
explicit claims. Every sealed output must be claimed by exactly one producing
action. Planning rejects implicit inheritance, unused declarations, undeclared
claims, and ambiguous outputs before execution. Mu needs a strict-routing mode
for these targets rather than its current convenience behavior of copying
target-level sealed inputs to emitted actions that omit their own map. This is
a least-privilege policy over mu's existing sealed execution and provider
machinery, not a second secret channel.

Field annotations classify the two channels: `@pudl(binding=plain)` permits a
catalog scalar projection, while `@pudl(binding=sealed)` identifies a sealed
input/output declaration. Classification is inherited through CUE unification,
unannotated declarations are denied, and conflicting classifications are
validation errors. PUDL validates references and compatibility but never calls
`resolve_secret` or `store_secret` itself.

For source-bound sealed inputs, the producer's converge-owned output declaration
is the sole authority for `ref` and `store_mode`; the consumer phase owns its
mu-visible input name and `delivery_mode`. Direct-ref inputs own their ref
because no producer model exists. The workspace is the sole authority for
`secrets.writable_refs`. Generated mu targets/actions receive those projections
without merging competing copies.

A sealed binding lowers to mu sealed-input/output references and execution
metadata, never to a concrete CUE scalar. A secret produced by an action stays
on the sealed-output side channel; it does not become an observation record.
Catalogs and reports may contain the declaration name, provider scheme, producer and
consumer identities, run IDs, status, and a redacted reference fingerprint,
but not the reference path or secret value.

The provider reference is not itself treated as secret plaintext: mu must carry
the full reference in its generated target/action configuration, action key,
and provider request so resolution and invalidation work correctly. PUDL's
durable surfaces expose only the scheme and fingerprint. The resolved value is
the datum excluded everywhere.

Canonical mu references are `docs/guide/protocol.md`,
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
- Add inherited `@pudl(binding=plain|sealed)` classification and conflict
  validation.
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
- Add observe-only and explicit `--converge` run-set modes.
- Replace request-level approval with canonical exact-plan identity and reject
  stale approval before mutation.
- Add versioned run-set reports plus linked, versioned member reports with
  typed binding evidence and mutation receipts.
- Add short ID-reservation, snapshot-selection, approval, and receipt
  transactions around an immutable plan object; never hold a catalog lock
  across mu.

### Phase 3 — sealed-reference wiring and execution

- Add phase-owned sealed input declarations and converge-owned sealed output
  declarations with CUE annotation validation. Reject populate sealed outputs
  structurally in v1.
- Support the exactly-one-of direct `ref` or cross-model `source` input union.
- Enforce producer-output ownership of provider reference/store mode,
  consumer-phase ownership of local name/delivery mode, and workspace ownership
  of write policy; reject every attempted override.
- Add strict explicit action-level routing in mu for PUDL-generated targets;
  reject implicit input fan-out, unused/undeclared claims, and any output not
  claimed by exactly one action.
- Lower consumer references and delivery modes to mu `sealed_inputs` and
  `sealed_input_modes` without resolving them in PUDL.
- Lower producer destinations and store modes to mu `sealed_outputs` and
  `sealed_output_modes`; pass an explicit `secrets.writable_refs` policy into
  every generated mu project that may write.
- Force mandatory exact-plan approval for every converging run-set whose model
  declares a sealed output, independent of `--require-approval`.
- Preserve mu's provider capability checks, pith taint boundary, file-delivery
  restriction, forced impurity, write-policy checks, and cleanup behavior.
- Add end-to-end tests proving values are absent from CUE, catalog rows, action
  keys, CAS, manifests, reports, stdout, stderr, and failure diagnostics.

### Deferred follow-ons

- whole-record and list-valued projections;
- arbitrary Datalog query bindings;
- automatic producer execution when a standalone consumer is missing data;
- explicit historical-version pinning or custom observation selection beyond
  the latest-successful convention;

## Acceptance criteria

The implementation is release-complete when these cases are specified and
tested:

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
9. A sealed producer writes through mu `sealed_outputs`/`store_secret`, and a
   consumer receives the same reference through `sealed_inputs` without PUDL
   observing the value.
10. Plain/sealed annotation mismatches fail before execution, and inherited or
    conflicting annotations behave deterministically.
11. Secret values are absent from every PUDL and mu persistence, diagnostic,
    cache, manifest, and process-output surface. Provider-reference paths occur
    only where mu needs them operationally; PUDL persists a fingerprint.
12. A forbidden sealed-output destination fails both plan-time and write-time
    policy checks through mu's `secrets.writable_refs` enforcement.
13. No mutation occurs before exact-plan approval, and changing any committed
    plan input invalidates approval and requires a new review.
14. A successful sealed write followed by failure is reported as completed
    partial state and is never represented as rolled back.
15. The first mutating failure prevents every unstarted mutation, including
    independent branches; dependents become `blocked`, unrelated unstarted
    members become `cancelled`, and only read-only diagnostics may continue.
16. A source-bound consumer cannot override its producer output's provider
    reference or store mode, and a model cannot override workspace writable-ref
    policy.
17. Phase-owned inputs accept exactly one direct `ref` or cross-model `source`;
    producer outputs are converge-owned, and desired/check fields cannot consume
    sealed declarations.
18. A multi-action plan exposes each sealed input only to actions that
    explicitly claim it, and routes each sealed output through exactly one
    explicitly claiming producer action.
19. Binding-derived `model_depends_on` facts reconcile atomically under
    `binding:<consumer>`, coexist with declared/heuristic provenance, and do not
    create per-run fact churn.
20. Versioned run-set/member reports round-trip complete plain provenance,
    metadata-only sealed provenance, redacted errors, and completed mutation
    receipts without any secret value or secret-value hash.
21. Concurrent catalog writes cannot redirect pinned inputs; pending approvals
    retain required snapshots, stale evidence invalidates approval, and a crash
    across an external-write/receipt boundary becomes `needs-verification`
    without automatic retry.

### Release-blocking verification matrix

V1 is not complete until a deterministic matrix passes in both PUDL and mu:

- CUE classification: inherited/conflicting/missing annotations and attempted
  plain/sealed coercion;
- plain selection: workspace isolation, pinned snapshots, age bounds,
  ambiguity, missing/non-leaf paths, type mismatch, and no forbidden fallback;
- sealed declarations: direct refs, producer outputs, exactly-one-of input
  validation, unique outputs, modes, phase restrictions, and provider
  capabilities;
- mu execution: strict action routing, env/file delivery, store modes,
  writable-ref policy at both enforcement points, cleanup, and secret
  non-leakage;
- graph behavior: missing producers, duplicate/self/cyclic edges, ordering,
  binding-fact reconciliation, and direct-ref non-edges;
- approval and failure: exact identity, stale rejection, no pre-approval
  mutation, global fail-fast, blocked/cancelled members, partial receipts,
  crash injection, and `needs-verification`;
- concurrency and retention: simultaneous observation/catalog writes,
  approval/resume races, immutable plan evidence, and pruning protection;
- reporting and compatibility: versioned JSON round trips, exact plain and
  metadata-only sealed provenance, redacted errors, unchanged unbound models,
  and unchanged single-model behavior.

The end-to-end fixture uses real mu planning and execution with a deterministic
fake provider implementing both `resolve_secret` and `store_secret`. Live
`pass`, Vault, or cloud-provider smokes are optional because credentials are not
a CI precondition; the fake-provider path is not optional.

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
### Review decision 6 — plain and sealed channels both ship in v1

Revised during follow-up design review: v1 is not plain-only. Plain values use
CUE elaboration; sensitive values use a distinct sealed-reference channel that
fully reuses mu `sealed_inputs`, `sealed_input_modes`, `sealed_outputs`,
`sealed_output_modes`, `resolve_secret`, `store_secret`, pith taint, and
`secrets.writable_refs`. CUE field annotations classify eligibility, with
deny-by-default and conflict rejection. Plaintext stays out of PUDL and mu's
persistent or diagnostic surfaces.

6. **Resolved:** ship both channels in v1; never represent a secret as a
   concrete CUE input or catalog value.

## Adversarial review status

Complete for the full sealed-I/O v1 boundary. The review decisions above are
implementation boundaries. Mutating run-sets and mandatory exact-plan approval
for sealed outputs are accepted; scheduling after an irreversible member
failure is globally fail-fast. Sealed authoring uses phase-owned inputs and
converge-owned outputs with no generic port or attachment layer, and
least-privilege action routing is explicit and mandatory. Canonical dependency
direction is consumer to producer. Binding-derived edges use the existing relation and a separate
authoritative fact source. Durable provenance uses versioned run-set and member
reports. Transactions are short and step-scoped around immutable plan evidence;
no lock spans external work. The deterministic PUDL+mu matrix is
release-blocking. Compatibility boundaries are reconciled with the dependency
substrate, historical roadmap, and project vision. Phase 0, Phase 1, mutating
Phase 2, and converge-backed Phase 3 are implemented. PUDL persists canonical
redacted exact plans, atomically creates and resolves approvals, revalidates
the normalized plan before each mutation, and invokes mu's guarded build so the
raw plan digest is compared before provider access and the same in-memory graph
is executed. Plan v2 also commits resolved plugin identities. PUDL fails globally
after a mutating error, records receipts and
`needs-verification`, durably records mutation intent before every external
apply, and lowers explicit strict action claims to mu. The matching mu change
landed in mu as `a334872` with boundary hardening in `6e757b8`; version-2 JSON
plan projection landed as `50921c5`, and the line was integrated by `3d44291`.
PUDL's repository smoke verifies the combined path through approval resume and
a fake provider.

The populate/approval contradiction is resolved for v1 by making sealed outputs
converge-only. Populate remains observation-only and may consume sealed inputs,
but its CUE arms and Go runtime projection expose no sealed-output write path.
This preserves the complete-plan approval boundary without staging secret
values or introducing partial approvals. An observe-only run may inspect a
model that declares a dormant converge output and still performs no mutation;
any mutating run that can execute that output remains exact-plan gated.

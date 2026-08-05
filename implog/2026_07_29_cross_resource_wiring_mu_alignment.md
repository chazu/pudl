# Cross-resource wiring design and mu secret alignment

**Date:** 2026-07-29
**Design:** `docs/design/2026-07-28-cross-resource-value-wiring.md`

## Outcome

The cross-resource value-wiring design was checked against mu's current secret
input/output contracts. A later design review promoted sealed input/output from
a deferred extension to a required v1 channel and reopened execution semantics
because sealed outputs are intentionally impure writes.

## Design decisions

- Scalar cross-resource values are elaborated through CUE before the model is
  decoded and rendered to mu.
- V1 sealed bindings must lower to mu `sealed_inputs` and
  `sealed_input_modes`, not concrete CUE values, catalog records, or reports.
- Execute pith must use `secret/get`, preserving mu/pith taint and redaction
  behavior; `env/get` remains the non-secret path.
- Producer-side secret capture must use `sealed_outputs` and the provider
  `store_secret` capability, with mu's existing `sealed_output_modes` and
  `secrets.writable_refs` policy.
- The provider boundary remains mu's `resolve_secret`/`store_secret` protocol,
  Go SDK handlers, and `SecretBackend`/`SecretPlugin` adapter.
- `@pudl(binding=plain|sealed)` classifies plain fields, phase-owned sealed
  inputs, and converge-owned sealed outputs. Unannotated fields fail closed,
  inherited annotations apply, and conflicts are errors.
- Sealed inputs choose exactly one direct provider ref or cross-model producer
  output. Outputs own references and store modes, consumer phases own local
  names and delivery modes, and workspace policy owns writable refs. No generic
  port/attachment layer exists. Populate outputs are excluded from v1 so all
  writes remain behind converge approval.
- PUDL-generated mu targets require strict explicit action-level routing.
  Inputs reach only explicitly claiming actions; outputs have exactly one
  claiming producer. Implicit fan-out, unused/undeclared claims, and ambiguous
  outputs fail during planning.
- Dependency direction remains consumer to producer in
  `model_depends_on(from, to)` for explicit, plain-binding, and sealed-binding
  edges. Scheduling reverses the edge internally; direct provider refs create
  no model dependency.
- Binding-derived edges persist under independently reconciled
  `binding:<consumer>` fact sources. Reconciliation is atomic after valid
  template loading, removal bitemporally invalidates stale binding edges, and
  coincident declared/heuristic/binding edges retain combined provenance
  without per-run facts.
- Reporting is a versioned two-level contract: one run-set report owns plan,
  approval, graph, and member outcomes; linked model reports own exact plain
  binding evidence, metadata-only sealed evidence, and typed external-mutation
  receipts. Secret values and secret-value hashes are never reported.
- Transactions are short and step-scoped: IDs and snapshot selections are
  atomic, immutable plan evidence is copied before external work, observations
  and receipts commit per step, and no catalog lock spans mu. Pending approvals
  retain pinned snapshots; missing evidence or digest drift makes them stale.
  A crash across an external mutation/receipt boundary requires verification
  and never retries automatically.
- V1 completion is gated on a deterministic cross-repo matrix covering CUE,
  selection, sealed policy/routing, graph behavior, approval/failure,
  concurrency/retention, reporting, and compatibility. Real mu execution uses
  a fake read/write secret provider in CI; live credentialed providers remain
  optional smokes.
- Documentation keeps the existing dependency relation/rules as substrate,
  supersedes its old no-value-flow/no-coordination claims, marks the Swamp
  roadmap as a historical predecessor, and clarifies that PUDL coordinates
  explicit model sets while mu alone executes side effects.
- PUDL carries provider references and redacted provenance only; it never
  resolves or persists secret values or provider-reference paths.
- A converging run-set whose model declares `sealed_outputs` requires mandatory
  exact-plan approval. Approval binds the full run-set and mu plans, including
  sealed references, modes, and write policy; plan drift invalidates approval
  before execution.
- Secret write policy is checked before approval, no mutation occurs before
  approval, and successful writes remain explicit partial state if a later
  action fails rather than being described as rolled back.
- Mutating run-sets are globally fail-fast: the first failure stops every
  unstarted mutation, marks dependency consumers `blocked`, marks unrelated
  unstarted members `cancelled`, and permits only read-only diagnostics to
  continue. Observe-only independent branches retain continue-on-failure.
- File delivery remains subject to mu's current `0600` temporary-file behavior
  and toolchain-sandbox limitation.
- Observation reuse follows a convention-over-configuration rule: current-run
  producer snapshots take precedence, standalone consumers use the latest
  successful scoped snapshot, and stricter age bounds stay at run policy level
  rather than in every CUE binding.

## Public API

No runtime or public API code changed. This document now records the
compatibility contract that the v1 implementation must reuse.

## Evidence

- Cross-checked the design against mu's `internal/config/schema.cue`, sealed
  input delivery documentation, secret write policy, plugin protocol, SDK
  guide, pith plugin guide, and pith sealed-I/O design.
- `git diff --check` passes for tracked changes; the new design and implog files
  are documentation additions with no trailing whitespace beyond intentional
  Markdown hard breaks.

# Architecture Improvement Report

**Date:** 2026-07-14 (design questions settled 2026-07-24)  
**Status:** Proposal with the first coordinator slice implemented; the six open
design questions are decided (D1-D6, revised after adversarial review) and a
15-entry defect register is recorded  
**Scope:** Highest-leverage improvements to the current PUDL codebase

## Executive summary

PUDL's product spine is now clear:

```text
#SystemModel
    |
    v
pudl run / ACUTE run coordination
    |             \
    v              v
mu observe/build   drift + checks
    |              |
    +------> CatalogDB <------+
                 |
       facts, snapshots, memberships,
       item schemas, run artifacts
```

The highest-leverage work is therefore at the seams around `pudl run`, ingestion,
catalog state, and workspace resolution. The recommended sequence is:

1. Make `pudl run` a testable ACUTE run coordinator with a deterministic fake-mu
   integration harness.
2. Finish the bounded-memory importer rewrite and retire the legacy importer
   layering.
3. Make observation snapshots first-class, scoped, and retainable.
4. Establish a single catalog transaction boundary and simplify the catalog
   schema/API.
5. Make workspace policy one explicit dependency shared by CLI and library code.
6. Compile and cache schema state per invocation, with identity metadata as a
   first-class contract.

The first item should come first because it exercises the other seams and gives
the project a reliable acceptance harness for future changes.

## Current architecture map

### ACUTE run path

- [`cmd/run.go`](../cmd/run.go) resolves a model, validates flags, chooses the
  drift mode, runs phases, renders a report, and persists status.
- [`cmd/run_populate.go`](../cmd/run_populate.go) stages a temporary mu project,
  invokes `mu observe` or an Ewe populator, and ingests results.
- [`cmd/run_drift.go`](../cmd/run_drift.go) handles differential observation.
- [`cmd/run_inventory.go`](../cmd/run_inventory.go) set-diffs desired resources
  against an observe snapshot or explicit catalog replay.
- [`cmd/run_converge.go`](../cmd/run_converge.go) invokes mu plan/apply commands,
  ingests manifests, and loops until clean or failure.
- [`cmd/run_report.go`](../cmd/run_report.go) renders the accumulated phase
  results as human-readable or JSON output.

### Import and observation path

- [`cmd/import.go`](../cmd/import.go) provides the CLI envelope-aware import
  entry point for regular, batch, and stdin input.
- [`internal/importer/enhanced_importer.go`](../internal/importer/enhanced_importer.go)
  computes content identity, parses data, infers schemas, writes raw artifacts,
  and creates catalog entries.
- [`internal/importer/importer.go`](../internal/importer/importer.go) remains the
  embedded legacy importer and owns much of the streaming/parser machinery.
- [`internal/mubridge/ingest.go`](../internal/mubridge/ingest.go) converts mu
  observations and manifests into catalog entries and snapshot memberships.

### State substrate

- [`internal/database/catalog.go`](../internal/database/catalog.go) owns the
  SQLite connection, catalog rows, query paths, and much of the row mapping.
- Facts, `current_facts`, FTS indexes, item schemas, collection memberships,
  run artifacts, statuses, and catalog EDB views all share the same database.
- [`internal/database/facts_tx.go`](../internal/database/facts_tx.go) already
  demonstrates an explicit transaction boundary for fact operations, but the
  wider run/import lifecycle is not yet covered by one boundary.

### Workspace and schema resolution

- [`internal/workspace`](../internal/workspace) discovers repository-local
  `.pudl` state and produces ordered schema paths.
- Command helpers such as `effectiveSchemaPaths` feed those paths into schema,
  inference, import, model, and run code.
- [`pkg/factstore/resolve.go`](../pkg/factstore/resolve.go) has a separate public
  workspace/rule resolution surface.

## Recommendation 1: make `pudl run` a testable ACUTE run coordinator

### Why this is the highest-leverage improvement

`pudl run` is the point where PUDL's domain concepts meet external effects:
desired resources, observers, snapshots, drift, checks, mu execution, manifests,
and convergence status. It currently works, but the run coordination is
concentrated in the Cobra command layer and crosses several persistence and
subprocess seams.

This does not move resource execution into PUDL. The authority split remains:

| Responsibility | Owner |
|---|---|
| Plugin and toolchain execution | mu |
| Provider mutations and action ordering within one mu invocation | mu and its plugins |
| Model resolution and `--only` scoping | PUDL |
| Choosing populate, drift, inventory, or converge | PUDL |
| Deciding when to observe, apply, and re-observe | PUDL |
| Iteration caps and convergence policy | PUDL |
| Drift/check interpretation and catalog/report persistence | PUDL |

The implementation target is therefore an internal PUDL module, not a new
daemon or execution engine. The Cobra command should translate CLI input and
render output; a run coordinator should own the PUDL lifecycle policy; a mu
adapter should continue invoking mu for each execution step.

The main risks are:

- phase behavior is difficult to test without real mu or infrastructure;
- catalog/status/fact writes are separate and often explicitly best-effort;
- the subprocess boundary is part of the domain behavior but is not represented
  as an injectable contract;
- partial apply behavior is reported, but not modeled as a durable run state;
- `--only` produces an `effectiveModel` for convergence, while the human plan is
  currently built from the original model;
- run identity, snapshot identity, manifest identity, and model status are not
  carried as one explicit session context.

### Proposed shape

The first refactor should extract run-coordination concepts without introducing
a large framework or duplicating mu's execution semantics:

```text
RunRequest
    |
    v
RunSession
    |
    +-- ModelResolver
    +-- ScopeResolver
    +-- Observer
    +-- DriftEvaluator
    +-- Reconciler
    +-- CheckRunner
    +-- RunStore
    +-- ReportRenderer
```

The interfaces should be small and internal. A sketch of the responsibilities:

```go
type RunRequest struct {
    Model       string
    Converge    bool
    Only        []string
    DryRun      bool
    FromCatalog bool
    MaxIters    int
}

type RunSession struct {
    RunID       string
    Model       *systemmodel.SystemModel
    Effective   *systemmodel.SystemModel
    Workspace   workspace.Context
    SnapshotID  string
}
```

The exact interfaces should be shaped around the current functions, not invented
ahead of the tests. The important seam is that the coordinator owns the effective
model, run lifecycle, and PUDL policy, while adapters own mu execution, SQLite,
and rendering details. The coordinator asks mu to execute an observation, plan,
or apply; it does not implement provider actions.

### Governing invariants

These should be written into tests before broad refactoring:

1. Unknown `--only` selectors fail before any external process or catalog write.
2. The effective scoped model is used consistently for planning, execution,
   report scope, resource promotion, and any scope-sensitive checks.
3. Every observation result is associated with exactly one run and snapshot.
4. A dry run does not mutate mu, catalog state, facts, memberships, or statuses.
5. A successful apply cannot produce `clean` without a verified re-observation.
6. A post-apply persistence failure cannot silently look like a clean run.
7. Mu stdout carrying a machine contract is separated from diagnostics on stderr.
8. A run that stops after a partial external apply remains visibly non-terminal
   until a later observation confirms its state.
9. Re-running the same observation or manifest is idempotent.

### Recommended implementation phases

#### 1A. Extract the pure run plan

Create a pure plan value from `RunRequest` and the resolved model. Use it for:

- flag validation;
- selector resolution;
- the printed plan;
- the effective model passed to every phase.

This immediately fixes the current plan/scope mismatch and gives tests a stable
object to assert against.

#### 1B. Add a mu runner adapter

Wrap `exec.Command("mu", ...)` behind a small interface with operations such as:

- `Observe`
- `Plan`
- `Apply`

The production adapter preserves the current subprocess behavior. A fake adapter
returns scripted observations, manifests, and failures without requiring mu,
Kubernetes, Docker, or network access. The adapter is a seam for testing PUDL's
coordination policy, not a replacement for mu's execution engine.

#### 1C. Add a run-session store

Record a run ID at the start and attach it to:

- the model run artifact;
- the observation snapshot;
- manifest-action entries;
- the final report/status.

Initially this can remain process-local for control flow while using durable IDs
for auditability. Full resume/recovery can follow once the state transitions are
well tested.

#### 1D. Add deterministic end-to-end coverage

The minimum matrix should cover:

| Scenario | Expected assertion |
|---|---|
| Observe-only differential run | mu observe called; no apply; report/status correct |
| Observe-only inventory run | populate snapshot is created and used for drift |
| Explicit `--from-catalog` | no live observe; only requested catalog scope is used |
| `--only` success | plan, apply, manifest, report, and promotion use the same scope |
| Unknown selector | fails before catalog or mu side effects |
| Converge to clean | apply manifest is `converging`, verified re-observe becomes `clean` |
| Apply failure | run is failed/non-clean and partial state is visible |
| Manifest persistence failure | apply result is not falsely reported as fully recorded |
| Repeated observation/manifest | no duplicate semantic state |

### Implemented first slice

The initial implementation preserves the authority split above:

- `internal/acute` owns the pure `RunPlan`, `--only` dependency closure, and
  observe/apply/re-observe convergence policy.
- `cmd` owns mu workspace preparation, catalog adapters, and rendering.
- mu still owns plugin/toolchain execution and provider mutations.
- fake executors test the coordinator without mu, Kubernetes, Docker, or network
  access.
- an apply whose manifest cannot be recorded produces `needs-verification` and
  maps to catalog status `unknown`; it cannot become `clean` from the same run.

The first slice attaches the session's audit ID to model, snapshot, and manifest
records. The remaining work is a durable run record (Defect 1) and broader
adapter coverage for every populate and observation path — *not* resumable
recovery, which D1 rejects.

### Settled decisions (2026-07-24)

The six design questions above are closed. Each decision is recorded with its
reasoning, because several are decisions *not* to build something.

These decisions were revised after an adversarial review on 2026-07-24 that
attacked the facts, the reasoning, and the completeness of an earlier draft.
D3 was rewritten (its original framing was falsified), D1 and D2 were reconciled
against each other, and D4/D5/D6 kept their conclusions but had their
justifications replaced. Where the earlier draft was wrong, the error is stated
rather than quietly removed.

**D1 — Run durability: no general resume. Build a durable run record with a
terminal marker.**
Re-running the ACUTE cycle is safe *because the cycle begins with an observe*:
observation, not stored state, is the arbiter of what to do next. A checkpoint
would only tell the runner what it already re-derives. So a general
checkpoint/restore state machine is not worth building, and PUDL cannot resume
*inside* an apply in any case — that is one `mu build` invocation and mu owns it.

Two obligations come with this decision, neither optional:

- **The iteration cap must become durable.** `MaxIterations` is per-process
  (`internal/acute/coordinator.go:103`), while `#SystemModel` carries
  `freshness.every` — runs are designed to be scheduled. A resource that
  oscillates (apply succeeds, drifts back) gets a cap of N *per run* and an
  unbounded global apply rate under any scheduler or crash-loop supervisor.
  Re-running is only safe if the halting guarantee survives it, so the cap must
  consult durable attempt history per model.
- **Re-running must stop destroying provenance.** See Defect 5.

The narrower resume primitive already exists and should not be rebuilt:
snapshots have durable IDs, and `--from-catalog --catalog-scope <id>` replays
drift against a prior snapshot. That replay is now correctly scoped and
correctly distrusted (Defect 3, fixed), but note it still compares against
observations that were never persisted on the converge path — see Defect 4.

**D2 — Post-apply persistence failure: `unknown` at the resource level, with the
reason carried on the run record.**
`converging` asserts that a receipt exists, when the lost receipt is the entire
problem. `failed` misdescribes the state: the apply succeeded, and the label
invites an operator to bypass the loop and re-apply by hand. `unknown` is the
honest remaining choice.

*Correction to the earlier draft:* it argued against adding a
`needs_verification` status by appeal to the removal of `converged`. That
precedent does not transfer. `converged` was removed because it was a **synonym**
of `clean` — two names, one state. `needs_verification` is not a synonym of
`unknown`; it is a strict refinement with a different operator action (verify a
mutation you know occurred, versus observe something never observed). The rule
was learned from a synonym case and was being applied to a distinction case.

The decision nevertheless stands, for a different reason: `unknown` is the
schema default for every new entry (`internal/database/catalog.go:288-290`), so
the collision it creates is real but is a *presentation* problem, and the run
record is the right place to solve it. This is conditional, not free — D2 is
**blocked on the run record from D1**, and until that lands the reason is
discarded at `cmd/run.go:247-249`. The sequencing below reflects the block.

Note that D1 and D2 are not in tension, though an earlier draft made them look
it. D1's "re-running is safe" is about the *loop*, which re-observes before it
applies. D2's "retry is unsafe" is about a *bare re-apply* that skips that
observation. Providers whose effects observation cannot see are outside PUDL's
convergence model regardless.

**D3 — Checks: evaluate them in converge runs, and scope them by constraint.**

*The earlier draft's framing was wrong and is withdrawn.* It claimed the
scoping question was malformed because checks are catalog-wide Datalog with
"nothing scope-shaped to pass." That is false on three counts:

- `Evaluate(db, rules, relation, constraints, scope)` takes a constraints map,
  and the run path **already uses it to scope a catalog-wide relation to the
  current model**: `cmd/run_depends.go:161-162` passes
  `map[string]interface{}{"from": m.Name}`. `runChecks` passing `nil` and a zero
  `TemporalScope` (`cmd/run_checks.go:79`) describes a call site, not a missing
  dimension.
- `catalog_entry_edb` exposes `run_id`, `collection_id`, `target`, `resource_id`
  and `origin`, so a check rule whose body binds `catalog_entry(run_id: $R, …)`
  is constrainable to `session.RunID` with **no schema change at all** — the
  option the draft skipped when it deferred classification for lack of evidence.
- The argument was circular: its evidence for "the question is malformed" was a
  description of the implementation under review.

The draft also contradicted Recommendation 3 of this same document, which asks
for snapshot-scoped catalog queries and tests proving stale snapshots cannot
affect inventory drift — the identical question, declared malformed for checks.

The decision that replaces it:

1. **Converge runs must evaluate checks.** Today the checks block
   (`cmd/run.go:192-202`) sits inside the `default:` observe-only branch, so a
   converge run reporting `clean` has evaluated no checks and claims more
   verification than it performed. (Precisely: checks are skipped whenever the
   converge branch is *taken*; `--converge` against a non-convergent model falls
   through to `default:` and does run them.)
2. **A fail-severity check must demote the converge verdict.** `runVerdict`
   derives status from `r.Converge` first, so hoisting checks alone would let a
   converge run persist `clean` while a `fail`-severity check failed.
3. **Checks are scoped by constraint, not by classification.** Pass the session
   run ID as a constraint where the rule's body admits it. This also fixes a
   silent inconsistency: inventory drift is computed against `pr.SnapshotID`
   while checks evaluate `current_facts` at wall-clock now, so within one run
   the drift verdict and the check verdicts are computed over different
   populations.
4. **Under `--only`, partition result *tuples*, not checks.** Where a result
   tuple carries a resource key resolvable against the effective model's
   selection, gate on in-scope tuples and report out-of-scope ones as advisory;
   where a rule head exposes no resource key, treat the check as global and gate
   normally. This needs no `#SystemModel` schema change and no waiting for
   evidence.
5. **`CheckResult` must carry the advisory/scope distinction.** Without it,
   `printChecks` renders `✗ … [fail] FAIL` while `anyFailSeverityFailed` quietly
   drops the exit code — an unexplained pass, which is exactly the "trains
   people to ignore checks" outcome the policy exists to avoid.

**D4 — Resource dependencies: ship the typed field; justify the relation
separately.**
The correctness half is unconditional and independent: declare `depends_on` in
the desired schema as a typed selector list with a defined resolution rule. It
needs no fact emission and no relation, and it removes the ambiguity class
behind Defect 2's dependency lookup.

*Correction to the earlier draft:* it justified this by reusing the cross-model
dependency machinery, and that economy does not materialize.

- **Identity is not a name.** `recordIdentity` (`cmd/run_inventory.go:24-53`)
  builds a compound, schema-relative key (`schema|field1/field2`) from the
  schema's `identity_fields`, so "reference the resource's identity name only"
  is not well-defined as written. The resolution rule must be stated against
  that compound identity.
- **Resource names are not unique across models,** so a flat
  `resource_depends_on(from, to)` collides two models' `nginx`. The relation
  needs a model dimension that the model-level relation does not have — same
  shape, different namespace regime.
- **Feeding the existing `impacted_by` means either corruption or duplication:**
  union resources into `model_depends_on` and `cyclic(model)` starts returning
  resources under an arg-key contract the rules file calls load-bearing, or
  duplicate all three recursive rules and reuse nothing.
- **The cardinality regime differs.** `reconcileEdges` loads every fact for the
  relation and diffs on every run; models number in the tens, while an inventory
  model's desired set can be thousands of resources with edges each.
- **The `--only` consumer must not read it.** Closure runs at plan time from the
  loaded model (`internal/acute/plan.go:133-153`) and must keep doing so —
  deriving converge scope from catalog facts would make `--only` depend on
  mutable state. So an emitted relation serves querying and reporting only.

Ship the typed field for correctness. Defer the relation under the same
"no evidence yet" rule applied elsewhere, and justify it on query value if it
is revived.

**D5 — Rollback: out of scope for V1, and never as inverse-action synthesis.**

*Correction to the earlier draft:* it concluded rollback was ruled out
"permanently" from the authority split. The premise supports only the narrower
claim. Rollback-by-reconvergence — select a prior desired revision, ask mu to
apply it — requires no knowledge of how to un-apply anything and uses the
identical authority split as a forward apply. The table in this document assigns
"deciding when to observe, apply, and re-observe" and "iteration caps and
convergence policy" to PUDL, and "converge to the last known-clean desired
revision" sits inside that. What the authority split actually forbids is PUDL
synthesizing inverse provider actions, which nobody proposed.

The honest limits on rollback-by-reconvergence, which are why it stays out of
V1 rather than out forever:

- **Convergence is not invertible for destructive or stateful resources.**
  Re-applying an old desired state recreates something with the same *name*, not
  the same *state*, once a converge has deleted a resource, revoked a cert, or
  terminated an instance with ephemeral storage. `inventorySetDiff` currently
  ignores extra observed records because "prune is deferred, matching host-
  converge V1" (`cmd/run_inventory.go:143-144`) — deferred, not rejected. When
  prune lands, git-revert-and-reconverge stops being a rollback for exactly the
  class of change where rollback matters.
- **It does not help when the failure is *in* converge.** `runConvergeLoop`
  already warns that a failed apply may leave a partial state with no rollback;
  re-converging to old desired hits the same failing apply at the same place.
  The capability that would help is "stop and restore last-clean," which the
  manifest records make feasible for the declarative case.

For declarative server-side apply this is a non-issue, and "the useful part of
rollback is knowing what a run changed, which manifests already answer" remains
the right reframing.

**D6 — Snapshot ownership: the session allocates the run ID; the observer
adapter returns the snapshot ID — with the snapshot row written on completion.**

*Correction to the earlier draft:* it justified adapter-returns by the property
"no snapshot row for an observation that never completed." The code does not
have that property. `IngestObserveResultsWithSnapshotRunID` creates the snapshot
collection entry *first* (`internal/mubridge/ingest.go:141-147`) and then
ingests members, returning the ID with an error on failure at record *i*;
`runPopulate` discards the ID on error. So a mid-ingest failure leaves an orphan
snapshot row in the catalog that the run never learns the ID of — the inverse of
the claimed guarantee, and the same invisibility Defect 1 describes, one level
down.

The split still stands, but the rule has to be stated properly, because the two
halves otherwise use opposite principles: run identity is pre-allocated so a
crash is visible, snapshot identity is post-allocated so a failure leaves no
row. The coherent rule is that **identifiers for work PUDL initiates are
pre-allocated, and rows are committed on completion.** PUDL decides when to
observe, so the snapshot ID should be allocated with the run and the snapshot
row written once ingest completes — giving the run record something to point at
for a partial snapshot, which adapter-returns-only cannot do.

### Defect register

Found while settling the decisions above and during adversarial review. Each was
confirmed against the code; those marked *(reproduced)* were demonstrated with a
temporary probe test. Ranked by blast radius.

#### Defect 1 — a crashed or interrupted run can leave a stale `clean` — **FIXED 2026-07-24**

A `runs` table now records every run: `StartRun` before any phase, `FinishRun`
from a `defer` so an early error return is still a *recorded* termination, and
`UnfinishedRuns` to surface rows left behind by a process that died without a
word. A converge run marks its model `unknown` up front, so a crash mid-converge
can no longer leave the previous run's `clean` standing. `persistRunStatus`
reports its failures instead of swallowing them. The in-process sibling below is
fixed by Defect 5's work: the observe-after-apply route now yields
`NeedsVerification` rather than an empty verdict. The original report follows.

*The earlier draft claimed a crashed run "records nothing at all." That is
refuted.* `recordModelInstance` (`cmd/run.go:89`) runs before any phase and
writes a run-ID-stamped `ObserveSnapshot` entry plus a model-instance row, and
`ingestConvergeManifest` writes run-ID-tagged `converging` rows immediately
after a successful apply. The scenario named — after apply, before re-observe —
is where the *most* is recorded.

The real defect is the absence of a run-level **terminal** marker, and it is
sharper than the withdrawn claim because the residue is actively misleading
rather than absent:

- `UpsertEntry`'s UPDATE set omits `status` (`internal/database/catalog.go:693-700`),
  so an interrupted run leaves the model-instance row holding the **previous**
  run's status. A stale `clean` survives a converge that mutated infrastructure.
- **In-process sibling (reproduced):** when `Observe()` fails *after* a
  successful apply, `acute.Converge` returns `{Outcome: "", Iterations: 1}` plus
  an error (`coordinator.go:80-83` returns before any outcome is assigned).
  `runVerdict` matches none of its cases and returns `""`; `persistRunStatus`
  returns immediately on `""`. The model row is left exactly as it was. No crash
  required.
- If `runConvergeLoop` fails before producing a report (e.g. workspace setup),
  `report.Converge` is nil and the same empty-verdict path is taken.
- Several observe-only paths `return err` directly (`cmd/run.go:157, 162, 168,
  175, 181, 188`), bypassing `persistRunStatus` entirely.
- `persistRunStatus` (`cmd/run.go:267`) discards both the open error and the
  update result (`_ = db.UpdateStatus(...)`).

Fix: write a run row at start, mark it terminal at end, treat a non-terminal row
from a dead process as such, and ensure every exit path either writes a verdict
or is visibly non-terminal. This also gives D2 somewhere to carry its reason and
closes invariants 6 and 8.

#### Defect 2 — `--only` selects resources the operator did not name — **FIXED 2026-07-24**

Both halves are fixed in `internal/acute/plan.go`: selector matching is now
key-class aware, and ambiguous resolutions are errors rather than silent picks.
Regression tests cover both failure modes, the legitimate type-selector set
match, the self-consistent dual-class case, and cycle termination. The original
report follows.


Two distinct bugs in `internal/acute/plan.go`, both from the same root:
`desiredSelectorValues` (`plan.go:192-215`) flattens `_schema`, `schema`,
`definition`, `name`, `id`, `path`, `kind`, `target` and `metadata.name` into a
single namespace.

**2a — dependency lookup picks the wrong resource (reproduced).** The scan at
`plan.go:138-143` takes the first candidate whose flattened set contains the
dependency string, then breaks. With desired
`[{name: decoy, kind: nginx}, {name: nginx, kind: Deployment}, {name: app,
depends_on: nginx}]` and `--only app`, `decoy` is pulled into the converge scope
because its *kind* is `nginx`, and the resource actually named `nginx` is left
out. Swapping declaration order flips the result to `[nginx app]`, so the bug is
deterministic and order-dependent rather than random — and silent.

**2b — top-level selector matching is over-inclusive (reproduced).** The
selection loop at `plan.go:123-131` has no `break`, so it selects *every*
resource whose flattened namespace contains the selector. `--only nginx` against
`[{name: decoy, kind: nginx}, {name: nginx, kind: Deployment}]` selects **both**.
Same production-mutation blast radius, opposite failure mode.

D4's typed-field fix addresses 2a only — it constrains what `depends_on` may
reference, not what `--only` selectors match against. **2b needs its own fix**
and remains after D4 lands. Until then both lookups should error on an ambiguous
match rather than resolve one.

A lesser aliasing issue sits alongside: `scoped := *model` with
`scoped.Desired = selected` (`plan.go:162-163`) gives a fresh slice whose
element maps are shared with the original, so a phase mutating a desired map
mutates both. Confirmed, not currently exercised.

#### Defect 3 — `--from-catalog` runs unscoped, and can manufacture a false `clean` — **FIXED 2026-07-24**

Both halves are fixed. `--from-catalog` now requires an explicit `--catalog-scope`
(a snapshot ID or ingest origin), rejected in `validateRunFlags` before any query;
`runInventoryDrift` independently refuses an empty scope so the invariant does not
depend on its caller. `ModelDriftResult` gained a `Verified` field, defaulting to
false, which `verifiedClean` and `runVerdict` both require — so a replay neither
promotes `converging` resources nor writes a model status. Regression tests cover
scope validation, the empty-scope refusal, and cross-scope isolation. The original
report follows.


Two compounding bugs on the catalog-replay path, both reaching a `clean` verdict
without a live observation.

**3a — no scope is applied.** `cmd/run.go:164-172` sets `scope` only inside
`if !flags.fromCatalog`, so `runInventoryDrift(db, "", …)` leaves `filter.Origin`
and `filter.CollectionID` empty (`run_inventory.go:175`) and queries **every**
`entry_type='observe'` item row in the catalog — every model, every host, every
snapshot, all time. A desired record can be satisfied by an observation from a
different model or host sharing schema and identity, and matches can be
arbitrarily stale. The acceptance matrix in this document asserts that
`--from-catalog` uses "only requested catalog scope"; no test covers
`scope == ""`.

**3b — it promotes `converging` to `clean` with zero observation.** `verifiedClean`
(`cmd/run.go:222-227`) is true for any `report.Drift.Clean`, including the
replay path, which drives both `persistRunStatus(…, "clean")` and
`promoteConvergingResources`. So `pudl run m --from-catalog` immediately after an
apply flips every resource that apply left `converging` to `clean`, off a
set-diff against records that may predate the apply. Invariant 5 requires a
verified re-observation; catalog replay is not one.

#### Defect 4 — observations that decide `clean` are never persisted — **FIXED 2026-07-24**

`observeDrift` now persists each differential observation via
`mubridge.RecordDriftObservation` — the verdict, the drifted resources and mu's
raw output — and `ModelDriftResult.ObservationID` / `acute.Observation.ObservationID`
carry the entry ID so a `clean` claim can be traced to stored evidence. The entry
type is `drift-observation`, deliberately **not** `observe`, so a drift verdict
can never be mistaken for an observed record by inventory drift. This covers both
the converge loop and the observe-only differential path. A dry run stores
nothing. The original report follows.

`observeDrift` (`cmd/run_drift.go:179-188`) execs `mu observe`, interprets drift
via a pure function, and returns — no catalog write. `acute.Observation` is
`{Clean bool; Details any}` with no snapshot ID. So the observation that decides
the verdict leaves zero evidence, and combined with Defect 1 a `clean` claim is
unfalsifiable after the fact. This is not a missing field but a missing
snapshot: there is nothing to associate.

This affects **both** the converge loop and the observe-only differential path
(`cmd/run.go:178-184` → `runDrift`), and the observe-only path is the worse of
the two because its `Clean` verdict directly drives `promoteConvergingResources`
in the default (non-converge) mode.

The `Details any` escape hatch is the same defect from the type side, but it is
a latent smell rather than a live bug: the only producer on this path
(`cmd/run_converge.go:80-86`) sets a `ModelDriftResult` value, so the assertion
at `run_converge.go:121` cannot currently fail.

#### Defect 5 — a lost apply receipt is masked on three of four exit routes — **FIXED 2026-07-24**

`ConvergeResult.NeedsVerification` is now orthogonal to `Outcome`, and every exit
route reaches it: the loop `break`s instead of returning early, so the verdict is
evaluated on all four. `runVerdict` checks it *before* the outcome, so a lost
receipt yields `unknown` rather than `failed` whatever else happened. Applying and
then failing to re-observe is treated as the same state, via a new
`OutcomeObserveError`; an observe failure before any apply stays an ordinary
failure. The original report follows.

`coordinator.go:129` converts `manifestFailure` into `OutcomeNeedsVerification`
only when the outcome is already `OutcomeClean`. On the cap-exhausted route the
outcome is `failed (cap_exhausted)` and the lost receipt is dropped
*(reproduced)*; `runVerdict` maps that to status **`failed`** — the exact status
D2 rejects for this case. The apply-error route (`coordinator.go:113-115`) and
the observe-error route (`coordinator.go:82`) return before the check and drop
it too.

The loop's cap arithmetic is otherwise correct: `MaxIterations: N` yields exactly
N applies and N+1 observations, with no off-by-one at `i >= request.MaxIterations`,
and a manifest failure on a *non-final* iteration latches correctly and
downgrades a later clean observation *(both reproduced)*.

#### Defect 6 — a dry run mutates catalog state, memberships and facts — **PARTIALLY FIXED 2026-07-24**

`recordModelInstance` and `reconcileModelDependencies` are now skipped on a dry
run, so no catalog entries, collection memberships, raw files or
`model_depends_on` facts are written; the run record and drift observations are
skipped too. Four of the five nouns in invariant 4 are therefore covered.

**Still open:** the scratch directory. `setupReconcileWorkspace` writes `mu.cue`
and the desired manifests into a temp subdir under the mu project root, and a dry
run genuinely needs them there — `mu build --plan` reads the config from disk, and
the project-embedded design is deliberate. Cleanup is deferred, so a *killed* run
leaves `pudl_run_*` behind, but that is equally true of a normal run and is not
specific to `--dry-run`. Recording it as fixed would be overclaiming: what remains
is a scratch-file lifecycle issue, not a catalog mutation.

Invariant 4 requires that a dry run mutate nothing. Two unconditional writes run
before the dry-run branch is reached:

- `cmd/run.go:89` `recordModelInstance` creates an ObserveSnapshot collection
  entry, an item entry, a collection membership row, and raw files under the
  data dir.
- `cmd/run.go:96` `reconcileModelDependencies` calls `AddFact`/`InvalidateFact`
  whenever `depends_on` differs from the recorded edges.

Only statuses and promotion are dry-run-guarded, so of the five nouns in
invariant 4 — mu, catalog state, facts, memberships, statuses — a dry run
violates four. Separately, `setupReconcileWorkspace` runs before `acute.Converge`
sees `DryRun` and `os.MkdirTemp(muRoot, "pudl_run_")` writes into the user's real
mu project tree; cleanup is deferred, so a killed dry run leaves `pudl_run_*`
behind.

#### Defect 7 — run/observation association is last-writer-wins

`ingestObserveRecord` (`internal/mubridge/ingest.go:286-289`) calls
`UpdateEntryRunID` on every deduplicated record, a bare
`UPDATE … SET run_id = ?`. An entry first observed by run A is rewritten to run
B on the next identical observation. The code comment states this is intended
("the run that most recently observed the item"), but invariant 3 requires
exactly one run, and the success measure "every run and observation can be
replayed by durable IDs" is false: querying run A under-reports. Snapshot
membership survives, so the two durable identifiers now disagree about the same
fact — and re-running, which D1 relies on, actively degrades the audit trail D1
exists to preserve.

#### Lower severity

| # | Defect | Location |
|---|---|---|
| 8 | Model-level status is written unscoped under `--only` — `persistRunStatus(model.Name, …)` marks the whole model `clean` when a subset converged. `promoteConvergingResources` *is* scope-aware; the model row is not, and `checkUpstreamFreshness` reads it. Invariant 2. | `cmd/run.go:216`, `:226` |
| 9 | `PromoteConvergingToClean`'s fallback promotes by bare `target` name with no model predicate, contradicting its own comment — two models declaring the same resource name share a key, so one model's clean drift promotes the other's pending rows. Reachable via untagged `ingest-manifest`. | `internal/database/catalog_status.go:44-62` |
| 10 | Checks receive `model`, not `effectiveModel`. Invariant 2, and it precedes the D3 work. | `cmd/run.go:193` |
| 11 | Snapshot content-hash dedup is dead code: the hashed payload contains a nanosecond-formatted `snapshot_id` and an RFC3339 `timestamp`, so the `GetEntry(contentHash)` lookup can never hit. Invariant 9 holds only at record level. | `internal/mubridge/ingest.go:183-216` |
| 12 | Manifest re-ingest is idempotent but returns before the per-action `UpdateStatus` loop and discards the new run ID, so the result carries the original run's ID. | `internal/mubridge/manifest.go:84-100` |
| 13 | A failed `GetCollectionByID` — not-found *or* DB error — degrades into an origin `LIKE` filter matching nothing, so every desired resource is reported `missing`. A DB error becomes a confident "everything is drifted". | `cmd/run_inventory.go:176-180` |
| 14 | No shared transaction within a run; the inventory path holds a reader open across a writer. Recommendation 4's target. | `cmd/run.go:155` / `cmd/run_populate.go:333` |
| 15 | Dead code: `ingestObserveOutput` and `ingestObserveOutputWithSnapshot` have no callers. | `cmd/run_populate.go:323`, `:328` |

#### Invariants that hold

Invariant 1 (unknown `--only` fails before side effects) and invariant 7 (mu
machine stdout separated from diagnostics on stderr) were verified to hold.

## Recommendation 2: finish the bounded-memory importer rewrite

The recent content-hash fix removed one unconditional large-file read, but the
pipeline still collects parsed objects into memory and retains a legacy importer
layer. Small JSON/YAML inputs also take a direct whole-file path.

The target design is a single staged stream:

```text
input -> hash + raw staging -> incremental decoder -> record sink
                                      |
                                      +-> schema/identity inference
                                      +-> collection membership
```

The first implementation should support NDJSON and large JSON arrays, preserve
the exact raw-byte content hash, and keep all-or-nothing collection semantics.
Then retire the embedded legacy importer and make memory-budget tests measure
peak memory rather than only elapsed time.

## Recommendation 3: make observation snapshots first-class

Observe ingestion currently creates timestamped collection entries and membership
rows. Add an explicit snapshot contract containing workspace, model, target, run
ID, source, creation time, and retention/currentness metadata.

Add:

- a current-snapshot lookup;
- snapshot-scoped catalog queries;
- retention/pruning of old snapshots;
- replay by snapshot ID;
- tests proving unrelated or stale snapshots cannot affect inventory drift.

This turns snapshot identity from a convention into a durable domain object.

## Recommendation 4: establish one catalog transaction boundary

`CatalogDB` now contains catalog entries, facts, current facts, FTS data, item
schemas, memberships, snapshots, run artifacts, and statuses. The relationships
are correct enough for the current paths, but the persistence boundary is spread
across multiple helpers and database handles.

The first slice should add a repository/session transaction that can atomically
record an observation or convergence step. It should also centralize catalog row
mapping and introduce an explicit migration version table. Once callers no longer
depend on the legacy collection columns, those columns can be retired in favor of
`collection_memberships` as the sole relationship source.

## Recommendation 5: make workspace policy one explicit dependency

Workspace schema precedence is implemented, but schema paths, rule paths, origin
filtering, model resolution, and catalog scope are still resolved through several
different helpers and globals.

Introduce one workspace policy value carrying:

- schema search paths;
- rule search paths;
- model/populator paths;
- effective origin;
- catalog scope;
- global/local mode.

Pass it into CLI services and public library constructors. Add contract tests for
local-only, global-only, shadowed, and nested-workspace cases.

## Recommendation 6: compile and cache schema state per invocation

Schema loading, CUE compilation, inheritance graphs, and identity metadata are
reconstructed repeatedly. Cache them within a command invocation, keyed by schema
repository revision or file fingerprints.

Use the same compiled schema state for:

- inference;
- validation;
- model loading;
- inventory identity resolution;
- schema commands.

This improves latency and prevents subtle differences where two phases resolve the
same workspace through different loaders.

## Suggested sequence

```text
1. RunSession + fake-mu harness
        |
        +--> 3. First-class snapshots
        |        |
        |        +--> 4. Catalog transaction boundary
        |
        +--> 5. Unified workspace policy
        |
        +--> 6. Schema compilation/cache

2. Bounded importer rewrite can proceed in parallel, but should land before
   large-file or high-volume production use.
```

The defect register under Recommendation 1 cuts across this sequence and should
be ordered by blast radius rather than by recommendation number. Two groups come
before anything else, because between them they cover every way the system can
currently mutate the wrong thing or claim a `clean` it did not verify:

1. ~~**Wrong-target mutation.** Defect 2 (`--only` mis-selects *and*
   over-selects) is the only defect that can mutate infrastructure the operator
   never named.~~ ✅ **DONE 2026-07-24** — selector matching is key-class aware
   and ambiguous resolutions error. D4's typed field remains worthwhile for
   dependency declaration, but is no longer load-bearing for safety.
2. **False `clean`.** Defects 3 (unscoped `--from-catalog`, replay-driven
   promotion), 1 (no terminal marker, stale status survives), 4 (unpersisted
   observations) and 5 (masked lost receipt) all end in a `clean` or `failed`
   verdict the evidence does not support. ✅ **Defect 3 DONE 2026-07-24** — it was
   a missing scope argument and a missing `verifiedClean` predicate, not a design
   change. Defects 1, 4 and 5 remain.
3. **Defect 1 then unblocks D2**, which cannot carry its reason until a run
   record exists, and closes invariants 6 and 8.
4. **Defect 4 is part of item 3**, first-class snapshots, and should be fixed
   with it rather than separately. **Defect 7** (run ID last-writer-wins) belongs
   with it, since D1's re-run-instead-of-resume position depends on re-running
   not degrading provenance.
5. **Defect 6** (dry-run impurity) is independent and small, but it invalidates
   `--dry-run` as a safe-inspection tool, which is how it is documented.

Defects 8-15 are quality and correctness cleanups that can follow the sequence
above in any order.

## Success measures

- A full ACUTE run can be tested without external mu or infrastructure.
- `--only` has one observable scope from plan through status promotion.
- Every run and observation can be replayed by durable IDs.
- A failed persistence step cannot create a false `clean` state.
- Large imports have a measured memory bound and no full-record accumulation.
- Workspace resolution is identical across CLI and library APIs.
- Schema loading occurs once per invocation and is shared by all phases.

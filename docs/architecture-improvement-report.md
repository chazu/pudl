# Architecture Improvement Report

**Date:** 2026-07-14  
**Status:** Proposal with the first coordinator slice implemented  
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

The remaining work in this recommendation is resumable recovery and broader
adapter coverage for every populate and observation path. The first slice now
attaches the session's audit ID to model, snapshot, and manifest records.

### Design questions for discussion

These are the decisions worth settling before implementing the durable session
store and broader adapter coverage:

1. **Run durability:** Should a run be resumable after the process exits, or is
   durable auditability enough for the first slice?
2. **Post-apply failure:** If mu applies successfully but catalog persistence
   fails, should the run be `failed`, `converging`, or a separate `unknown` state?
3. **Scope of checks:** With `--only`, should model checks evaluate the full model,
   only the effective resources, or be explicitly classified as global/scoped?
4. **Dependency closure:** Should resource dependencies be represented only in
   desired data, or should the system model schema expose a first-class resource
   dependency relation?
5. **Rollback:** The current contract has no rollback. Is the desired guarantee
   idempotent re-observation and retry, or should mu eventually expose compensation?
6. **Snapshot ownership:** Should the run session create the snapshot, or should
   the observer adapter return one as part of its result?

### Recommended defaults for the first slice

- Make runs durable for audit, but not resumable yet.
- Add a distinct `unknown`/`needs-verification` state if persistence fails after
  an external apply; never infer `clean`.
- Use the effective model for convergence and scope-sensitive reporting. Keep
  explicitly global checks marked as global.
- Keep dependency closure in the session plan for now; defer a schema redesign.
- Treat idempotent re-observation as the recovery mechanism; leave rollback to mu.
- Have the observer adapter return a typed snapshot result containing its ID and
  ownership metadata.

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

## Success measures

- A full ACUTE run can be tested without external mu or infrastructure.
- `--only` has one observable scope from plan through status promotion.
- Every run and observation can be replayed by durable IDs.
- A failed persistence step cannot create a false `clean` state.
- Large imports have a measured memory bound and no full-record accumulation.
- Workspace resolution is identical across CLI and library APIs.
- Schema loading occurs once per invocation and is shared by all phases.

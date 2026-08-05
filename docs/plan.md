# PUDL Development Plan

Living document tracking what is built and what comes next.

## What's Built

### Repository-local run-set kick-the-tires (2026-08-05)

The in-repository `.pudl` fixture now exercises real mu observe, planning,
approval, provider, recovery, and concurrency boundaries while keeping every
fixture and runtime artifact local. Live execution confirmed plain
producer-to-consumer ordering, exact current-run snapshot binding, standalone
reuse, structured fail-fast reports, durable non-sealed approval/resume/reject,
stale-plan rejection, single-model sealed provider I/O, write-policy denial,
crash `needs-verification`, and concurrent catalog safety.

The sweep fixed four PUDL gaps: `model validate` now accepts structurally valid
bound templates, requested malformed models retain their actual validation
error instead of becoming “not found,” repo init/doctor own the supported
`populators/` directory, and single-run approval requests preserve `--mu-root`
across process resume. See
[`implog/2026_08_05_repository_kick_tires.md`](../implog/2026_08_05_repository_kick_tires.md).

One release blocker remains open as `pudl-olm`: mu v0.3.3 receives and executes
sealed target declarations but omits action-level sealed claims from
`build --plan --json`. PUDL therefore cannot validate the strict exact plan and
fails sealed mutating run-sets closed before approval. Existing fake-runner
coverage proves the intended contract, not the live bridge. Plain run-sets,
non-sealed exact-plan approvals, and single-model sealed convergence remain
executable.

### Swamp parity utility/UX slices (2026-07-27)

Done — see [`docs/design/2026-07-27-swamp-parity-roadmap.md`](design/2026-07-27-swamp-parity-roadmap.md)
and [`implog/2026_07_27_swamp_parity_slices.md`](../implog/2026_07_27_swamp_parity_slices.md).
PUDL now has scaffold-first model/rule/populator commands, ad-hoc observe-only
plugin runs, machine-readable help/model descriptions, durable run reports,
request-level converge approvals with resume/reject, troubleshooting/memory
guides, and an advisory raw-infrastructure hook. Mu's Kubernetes plugin has an
explicit typed inventory observe mode. Observe ingestion now refuses dangling
schema refs and falls back safely to inference or the generic observe schema.
The convergence stocktake also closed the catalog-plugin reconcile path: desired
manifest sources are absolute within the mu project root, mu accepts those
contained absolute action inputs, and terminal convergence errors survive in
durable run reports. Typed projection/validation alignment is now closed for
the shipped path: schema loading preserves schema-root-relative package paths,
explicit plugin/bridge assignments validate against their assigned CUE schema,
and fixed-point inference is reserved for genuinely inferred imports. The
mixed 41-entry acceptance fixture now passes both `pudl validate --all` and
`pudl verify` with zero invalid entries or mismatches. Cross-model value wiring
now has an accepted design, including full v1 integration with mu's sealed-I/O
primitives and convention-over-configuration observation reuse. The retained
template seam, standalone plain-scalar resolver, exact `run-set` execution,
canonical non-sealed mutation approval, and fail-fast receipts are implemented.
Producer-first scheduling, current-run snapshot pinning, stale-plan rejection,
redacted sealed provenance, and linked durable reports cover the executable
orchestration slice. The strict sealed run-set logic is implemented at PUDL's
coordination boundary but remains fail-closed against real mu plan output as
described above. Living docs and CLI help describe that boundary; init/repair
derives its owned file set from every
embedded schema plus `#SystemModel`, and selected non-secret built-in resource
handles expose explicit plain-binding annotations. One contract inconsistency remains: an action-backed populate cannot
both finish the required pre-approval observation and defer its sealed write
until after approval. PUDL fails that combination closed pending a design
decision. Single-model converge-phase sealed production and consumption work;
their strict cross-model run-set plan is blocked by `pudl-olm`.
Bindings do not carry
freshness fields: orchestrated runs prefer current-run observations, standalone
runs use the latest successful scoped observation, and stricter age bounds
belong to run policy. See
[`docs/design/2026-07-28-cross-resource-value-wiring.md`](design/2026-07-28-cross-resource-value-wiring.md)
and [`implog/2026_07_29_cross_resource_wiring_mu_alignment.md`](../implog/2026_07_29_cross_resource_wiring_mu_alignment.md).

### Adversarial review closure (2026-07-14)

- `pudl run --converge --only` now validates exact resource selectors before
  side effects and includes declared resource dependencies.
- Inventory runs compare against the current observe snapshot; `--from-catalog`
  is the explicit replay override.
- Content hashing, collection import, observe snapshots, and typed envelopes use
  bounded or shared paths; collection membership is normalized and deletion is
  safe for shared items.
- Workspace schema resolution is local-first with global fallback. Repository
  catalogs and every mutable artifact are now rooted under `.pudl/`, while
  global mode remains isolated under `~/.pudl/`. The CI workflow runs build,
  test, vet, generated-skill, race, and optional smoke gates.
- Review tickets are tracked in Beads Rust under `.beads/`; all tickets from this
  review are closed.

The core pipeline is stable and tested. Execution-related features (models, methods, workflows, Glojure runtime, artifacts) were implemented in Phases 1-8, then extracted into **mu** as a separate tool. What remains in pudl is the knowledge layer. Residual execution CLI surface (`pudl data search/latest`, `drift check --method`, Glojure adapter) has been removed; some internal artifacts (database fields, CUE model schemas) remain for future cleanup.

### Data Lake
- Multi-format import (JSON, YAML, CSV, NDJSON) with automatic format detection
- Collection support for NDJSON plus typed envelope metadata
- SQLite catalog with query, filter, pagination, and provenance tracking
- Content-based identity (SHA256, proquint display) for deduplication
- Resource identity from schema identity fields with version tracking, namespaced
  by the inheritance-family root (stable under reinference and policy refinement;
  `pudl migrate identity --recompute` migrates existing entries)

### Schema System
- CUE-based schema inference using heuristics and CUE unification
- Schema generation from imported data (`pudl schema new`)
- Schema name normalization to canonical `<package>.#<Definition>` format
- Built-in schemas and rules, including core, AWS, Git, Kubernetes, Linux, mu,
  nous, convergence rules, and `pudl/systemmodel.#SystemModel`; init repair is
  derived from the embedded file set so newly shipped built-ins are included
- Git-backed schema repository with status/commit/log
- Pluggable type patterns (AWS, Kubernetes, GitLab)
- Component/schema boundary: `#`-definitions with no `_pudl` block are treated as
  reusable *components* (inert to inference, not phantom schemas); list-type
  collection schemas are exempt. See `implog/2026_06_15_component_schema_boundary.md`
  and `docs/issues/git-repository-decomposed-resources.md` (D1).
- Built-in git-repository schema family (`pudl/git`): base `#GitRepository`
  (identity `["name"]`, inline `#GitRemote`/`#GitBranch` components) plus
  `#GitHubRepository`/`#GitLabRepository` specializations. C1–C4 (fan-out/reconcile
  for separate child resources) deferred. See
  `implog/2026_06_15_git_repository_schema_family.md`.

### Validation and Verification
- CUE structural validation (`pudl validate`)
- Model validation (`pudl model validate`)
- Fixed-point verification (`pudl verify`) confirming schema assignment stability

### System Models
- `#SystemModel` loading and structural validation
- Desired-state drift detection, checks, reporting, and optional convergence
- Cross-model dependency facts and model/run commands

### Drift Detection
- JSON deep diff comparing desired vs observed state
- Inventory drift over the current observe snapshot, or explicit catalog replay
- Field-level diffing with added/removed/changed tracking and run reports

### Catalog Layer
- Bootstrap `catalog.cue` registering core types
- `pudl catalog` browsing registered schema types
- Extensible by user-defined entries

### Mu Bridge
- `pudl mu ingest-observe` records timestamped observe snapshots
- `pudl mu ingest-manifest` records per-action convergence results
- `pudl run --converge` renders desired state and delegates execution to mu
- Catalog-installed digest plugins synchronize package-owned `mu/...` schemas
  and validate PUDL mappings automatically; missing bundles report the exact
  `mu plugin install NAME[@VERSION]` repair command.

### ACUTE Feedback Loop
- `pudl mu ingest-observe` — ingest mu observe results as live state for drift detection
- `pudl mu ingest-manifest` — ingest mu build manifests, track per-action results
- `pudl status` — per-model/resource convergence status (unknown/drifted/converging/clean/failed); unknown also means an apply receipt needs verification
- Status column on catalog entries, updated through the full ACUTE cycle
- Architecture: [`docs/acute-loop-architecture.md`](acute-loop-architecture.md)

### Per-Repo Workspaces
- `pudl repo init` idempotently creates or repairs `.pudl/workspace.cue`, local
  config/data directories, a CUE module, every built-in schema, models, and
  definitions
- Workspace discovery walks up from cwd looking for `.pudl/workspace.cue`
- Repository commands use `.pudl/data/sqlite/catalog.db`; no mutable catalog,
  report, approval, snapshot, fact, raw-data, or metadata state leaks globally
- Catalog queries remain scoped by workspace origin (`--all-workspaces` bypasses
  the origin filter within the repository's catalog)
- Multi-path schema resolution with per-repo shadowing of global schemas
- Imports within a workspace auto-tagged with workspace name as origin

### Agent Integration
- `pudl prime` outputs a structured prompt teaching agents how to use pudl
- `pudl guide` provides topic-based reference guides for agents and humans
  (overview, import, schemas, facts, datalog, models, mu, agents,
  troubleshooting, memory)
- `pudl repo init` creates `.pudl/` with workspace.cue and installs Claude skills

### Documentation
- Reorganized docs: user-facing guides in `docs/`, active work in `docs/beads/`, research in `docs/research/`, completed plans in `docs/archive/`
- `docs/README.md` index covers all subdirectories

### Infrastructure
- `pudl doctor` with directory structure validation
- Database migrations (idempotent, run on every open)

---

## Public API Extraction (fact store + datalog)

Goal: let external Go applications interact with pudl data stores (global `~/.pudl`
and repo-scoped `.pudl/`) through `pkg/factstore` and `pkg/eval` **without importing
`pudl/internal/*`**. The `internal/` rule already blocks external import of internal
packages; the work is making the `pkg/` facade complete and non-leaky.

### Phase 1 — Extraction + dead-code nuke (done)

The live query path (partition → SQL → recursive fallback) is currently inline in
`cmd/query.go` and not reusable. `pkg/eval` only exposes the legacy in-memory
evaluator, which is dead code. Plan:

1. `internal/datalog/match.go` — move shared helpers (`matchConstraints`,
   `valuesEqual`, `toFloat64`) out of `eval.go` before deletion (`sql_eval.go` uses
   `matchConstraints`).
2. `internal/datalog/query.go` — `Evaluate(db, rules, relation, constraints, scope)`,
   the single orchestrator; `cmd/query.go` calls it (behavior unchanged).
3. Delete dead code: `eval.go`, `eval_test.go`, `edb.go`, `index.go`; trim
   `Binding`/`Apply` from `types.go` (keep `ParseTerm` — loader uses it).
4. `pkg/eval` — strip to rules + types: `Rule/Atom/Term/Tuple/Var/Val`,
   `LoadRulesFromPaths`, `ParseRulesFromSource`. Remove `EDB`/`NewEvaluator`/
   `New*EDB`.
5. `pkg/factstore` — drop leaky `DB()`; add `QueryOptions` + `Store.Query` (calls
   `datalog.Evaluate`); re-export `Rule`/`Tuple` so query-only callers need only
   `factstore`.
6. `pkg/factstore/resolve.go` — `GlobalDir()`, `DiscoverWorkspace(cwd)` →
   `{RepoDir, RulePaths}`, wrapping `internal/config` + `internal/workspace` (no
   internal types in signatures).
7. Tests (factstore query covering SQL + recursive, eval parse, resolve), full
   suite green, implog.

Decisions locked: `Query` lives on `factstore.Store`; Phase 1 ships before Phase 2.

Done 2026-06-07 — see `implog/2026_06_07_public_api_extraction.md`. Also fixed a
latent recursion-routing bug in the query path: relations with both a base and a
recursive rule previously returned only the base tuples.

### Phase 3 — Transactional check-and-write (done)

Done 2026-06-12 — see `implog/2026_06_12_fact_tx_and_dlktk_schema.md`.
`factstore.Store.Transact(fn func(*Tx) error)` runs reads and writes inside one
immediate-mode SQLite transaction (write lock held from BEGIN), closing the
TOCTOU race between an invariant check and its write for external consumers
(dlktk's multi-process move legality being the motivating case). The `Tx`
handle exposes `AddFact`/`RetractFact`/`InvalidateFact`/`QueryFacts`/
`FactHistory`; an error from the callback rolls everything back. The same
change registered `pudl/dlktk` as a built-in bootstrap schema package typing
the args of every `dlktk/*` relation.

### Phase 2 — Catalog-as-datalog bridge (done)

Done 2026-06-08 — see `implog/2026_06_08_catalog_datalog_bridge.md`. `catalog_entry`
is a built-in EDB relation (backed by the `catalog_entry_edb` view) usable as a rule
body atom; rules can join facts against catalog data through `Store.Query`. The
relation name is reserved at `AddFact`. Direct querying of `catalog_entry` (no rule)
returns an explicit join-only error rather than a silent empty result, and
`factstore.Store.ListCatalog` provides typed catalog access. Public API documented
in `docs/library-api.md`. Design notes below.



Let datalog query the catalog alongside facts via one `Store.Query` API: expose the
catalog as a `catalog_entry` relation usable as a rule body atom, so rules can join
facts against catalog data.

**Mechanism.** The compiler already handles native-column tables via
`CompileOptions.TableOverrides`: for an overridden relation it accesses
`alias."col"` directly (no `json_extract`), skips the `relation = ?` filter, and
skips temporal filters. So no JSON re-shaping is needed for column access — the
earlier "facts are JSON / catalog is columns → need a view" reasoning was wrong on
that point. The recursive path (`fixpointLoop`) already threads `TableOverrides` for
`_delta_` temp tables; the catalog override just merges in.

**Design decisions (locked):**

- **Q1 — Curated SQL view, not a raw table override.** Point `catalog_entry` at a
  view (e.g. `catalog_entry_edb`), not at `catalog_entries` directly. Reason is
  interface design, not column access: the physical columns are too internal/unstable
  (`item_id`, `stored_path`, `metadata_path`, `record_count`) to be the public
  datalog interface. The view renames (`item_id`→`resource_id`), hides internals, and
  pins the exposed/migrated column set explicitly.
- **Q2 — Rule-body-only first.** `catalog_entry` works as a body atom inside rules,
  not as a direct query target. A bare `Store.Query{Relation:"catalog_entry"}` with
  no rule hits `fallbackEDB` (facts table) and returns nothing; direct-query support
  (a `fallbackEDB` branch selecting from the view) is a later add if wanted.
- **Q3 — Built-in override map, hardcoded.** A package-level
  `builtinEDBTables = {"catalog_entry": "catalog_entry_edb"}` injected at the three
  compile sites: `sql_eval.Query` (switch `Compile` → `CompileWithOptions`),
  `recursive.seedBase`, and `recursive.fixpointLoop` (merge into the existing
  `_delta_` override map — no key conflict, since `catalog_entry` is never a derived
  relation).
- **Q4 — Reserve the relation name; owner `database`.** Keep the clean name
  `catalog_entry`; `database` owns a `builtinEDBRelations` set and `AddFact` rejects
  those relations with a clear error (prevents silent shadowing of user facts by the
  view). The datalog override map must reference the same names — a test asserts the
  two stay in sync.

**Implementation outline:**

1. `database`: add `catalog_entry_edb` view to the idempotent migrations (renaming
   `item_id`→`resource_id`, excluding internal columns); add `builtinEDBRelations`
   set + guard in `AddFact`.
2. `datalog`: `builtinEDBTables` map; inject at the three compile sites; sync test
   against `database.builtinEDBRelations`.
3. Tests: rule with a `catalog_entry` body atom joined against a fact relation;
   recursive rule referencing `catalog_entry`; temporal query with a catalog atom
   present (catalog atemporal, facts scoped); `AddFact` rejects reserved relation.

**Edge items to handle/doc:** nullable view columns bind to nil (joins on NULL never
match — expected); numeric constraints from the CLI are strings, rely on SQLite type
affinity for numeric view columns.

## What's Next

Potential future work, roughly ordered by value.

### Cross-Resource Value Wiring — Implementation at the Populate/Approval Boundary

Status: Phase 0, Phase 1, mutating Phase 2, and converge-backed Phase 3 are
implemented and verified. Discovery retains incomplete
`ModelTemplate` values;
plain inputs resolve through exact successful, workspace-scoped snapshots;
schema-relative identity and RFC 6901 scalar projection are fail-closed; CUE
performs final type validation; `pudl run` accepts `--max-observation-age`; and
versioned member reports carry exact binding evidence. Structured run completion
now prevents failed, unfinished, registration-only, and legacy snapshots from
being reused. Observation ingest persists declared schema identity so selectors
do not rely on heuristics.

The new `pudl run-set <model>...` surface rejects incomplete exact sets,
duplicates, self-dependencies, and cycles before execution; orders producers
first with lexical tie-breaking; blocks transitive consumers after failure;
continues independent observe-only branches; and persists a versioned run-set
report linked to versioned member reports. `pudl run-set report [id]` retrieves
that orchestration record. Binding facts reconcile atomically under their own
provenance source, and dry-run binding reads use a non-migrating read-only
catalog connection.

Mutating run-sets now plan every member before applying, persist one immutable
redacted plan identity for executable non-sealed plans, reject stale resumes,
stop globally after the first mutation failure, and record completed receipts
or `needs-verification`. A durable write-ahead mutation intent now
marks every external apply uncertain until its manifest receipt commits, so a
process loss cannot silently invite an automatic retry. Generated mu targets
request strict action-level sealed claims, workspace-owned write policy, delayed
provider resolution, and metadata-only PUDL evidence. PUDL's coordination tests
cover synthetic sealed action claims, and mu has a real fake-provider execution
fixture; the real JSON-plan bridge between them remains blocked by `pudl-olm`.

The populate/approval boundary is now resolved: sealed outputs are
converge-only in v1. Populate may consume sealed inputs for authenticated
observation, but the CUE schema, Go projection, and generated mu populate
project expose no write path. Observe-only runs may inspect a model with a
dormant converge output without mutation. The accepted contract automatically
gates a mutating run that can execute the output, but real sealed run-sets fail
closed before that gate until `pudl-olm` is resolved. The working design is
[`docs/design/2026-07-28-cross-resource-value-wiring.md`](design/2026-07-28-cross-resource-value-wiring.md),
with mu secret-input/output compatibility recorded in
[`implog/2026_07_29_cross_resource_wiring_mu_alignment.md`](../implog/2026_07_29_cross_resource_wiring_mu_alignment.md).

The first implementation slice is intentionally limited to scalar leaves. A
consumer model binds an input slot to a typed scalar leaf from a producer's
observation, using an exact `(model, schema, identity, path)` selector. The
binding itself does not carry freshness or TTL configuration. By convention,
an orchestrated run prefers the producer's current successful observation;
standalone resolution uses the latest successful observation in the current
scope. Missing or invalid values fail closed, and stricter age bounds belong
to run/operator policy rather than the CUE binding API. Secret-valued wiring
uses a separate sealed-reference channel. Its single-model path reuses mu's
sealed-input, sealed-output, secret-provider, taint, and write-policy
primitives; cross-model exact planning remains blocked by `pudl-olm`. Secret
values never become CUE inputs or catalog values.

The following questions surfaced in the final adversarial review and are
resolved inline as the implementation contract.

#### Execution boundary

1. **What is the multi-model run surface?** `pudl run` currently resolves and
   executes one model. The wiring design assumes a producer and consumer can
   participate in one orchestrated run, with producer-first ordering. Define
   the run-set request (CLI and/or library), model selection, ordering,
   cycle handling, per-model run identity, snapshot ownership, and failure
   propagation. Also reconcile this with the existing dependency contract,
   where `depends_on` records ordering/impact facts and mu owns orchestration.

   **Resolved:** PUDL owns model-level coordination through a separate
   `pudl run-set <model> <model>...` surface backed by an exact `RunSetRequest`;
   mu continues to execute one elaborated model's internal graph at a time.
   Bindings require their producers in the explicit set, while `depends_on`
   targets outside it remain advisory. Binding and in-set `depends_on` edges
   determine producer-first order with lexical tie-breaking. Duplicate models,
   self-dependencies, and cycles fail preflight. The operation has one run-set
   ID plus per-model run and snapshot IDs. Observe-only failures block
   transitive consumers while independent branches continue. Mutating failures
   globally stop new mutations: dependents become `blocked`, unrelated
   unstarted members become `cancelled`, and any non-successful member makes
   the command fail.

2. **Which execution phases participate?** Decide whether value production is
   limited to observation/populate, or whether checks, desired-state
   evaluation, and convergence are part of the same dependency run. This
   determines when a producer is considered available and whether a later
   convergence failure invalidates an otherwise usable observation.

   **Resolved:** `pudl run-set` remains observe-only unless `--converge` is
   present. A producer becomes available only after its complete lifecycle for
   that mode succeeds. Mutating run-sets finish read-only observation,
   elaboration, policy validation, and planning before executing anything. Any
   converge-owned sealed output makes exact-plan approval mandatory during a
   mutating run regardless of `--require-approval`; ordinary convergence
   retains the explicit flag's existing policy. The approval digest commits to
   the complete model graph,
   pinned snapshots, resolved plain inputs, desired/config projections, plugin
   and action identities, sealed references and modes, writable-ref policy,
   and converge options, but never a resolved secret value. Approval/resume
   regenerates the plan; any difference invalidates approval with no mutation.
   Forbidden writes fail before an approval request. Successful external
   writes are not rolled back if a later member fails and must be reported as
   completed partial state. The first mutating failure stops every unstarted
   mutation, including independent branches; only read-only re-observation,
   verification, and reporting may continue. V1 has no
   `--continue-on-error` override.

#### Observation selection and reuse

3. **What does “successful observation” mean?** Observe snapshots currently
   record provenance and timestamps but do not record an explicit status. Define
   whether success means successful ingestion, successful populate, or a
   successful complete model run, then encode or query that definition
   consistently.

   **Resolved:** success means the owning model completed the full
   observe-only lifecycle defined above. `runs` gains a structured
   `running`/`succeeded`/`failed`/`blocked` completion status, separate from its
   drift verdict. A snapshot is eligible only by joining matching model/run
   provenance to a `succeeded` run and confirming it is a real typed
   observation. Registration and differential-drift artifacts, failed or
   unfinished runs, and legacy rows without provable structured success are
   ineligible. Failure to persist success blocks consumers.

4. **What is the scope of “latest”?** Snapshot selection must be scoped to the
   current workspace/origin as well as the producer model, schema, identity,
   and path. The selected immutable snapshot ID should be pinned before value
   lookup so a concurrent observation cannot change the consumer's inputs.

   **Resolved:** a run set uses the exact successful snapshot owned by its
   producer member. A standalone run selects the newest eligible snapshot for
   the exact producer model and current `EffectiveOrigin` workspace, with no
   cross-workspace or `global` fallback. Origin/source remain system provenance,
   not binding fields. PUDL pins the snapshot before resolving schema, identity,
   or path and uses one consistent catalog read view. Absence from that newest
   snapshot fails; resolution never searches older snapshots for a resource
   that may have been deleted.

5. **Who owns an optional age bound?** The existing `#SystemModel.freshness`
   field describes loop cadence, not observation age. If an age bound is
   retained, define it as a separate run/operator policy, its CLI/config API,
   report representation, and precedence. It may tighten the conventional
   selector, never loosen it. Otherwise defer age bounds until the selector
   contract is established.

   **Resolved:** retain an explicit `--max-observation-age <duration>` policy
   on `pudl run` and `pudl run-set`, backed by an optional library request
   field. The first release has no config-file default, binding field, or reuse
   of model `freshness`. Age is evaluated once at consumer elaboration against
   the pinned snapshot and applies to current-run and reused snapshots alike.
   Rejection never changes the selected snapshot or starts a producer. Reports
   include snapshot/evaluation times, age, bound, and status; future policy
   layers combine by taking the smallest bound.

6. **What happens when the current-run producer fails?** Specify that a failed
   producer cannot silently fall back to an older observation when the
   consumer is part of the same orchestrated run. Define the corresponding
   standalone behavior when there is no current-run producer.

   **Resolved:** a current-run producer that fails, is blocked, fails a
   fail-severity check, or produces no eligible snapshot blocks all dependent
   consumers with no historical fallback. A standalone consumer may reuse the
   newest eligible successful snapshot even if a newer producer attempt failed,
   subject to workspace and age policy, but its report must disclose both the
   reuse and the failed attempt. No eligible or sufficiently recent snapshot
   means failure before mu planning; PUDL never starts the producer implicitly.

#### Binding and CUE contract

7. **What is the scalar path grammar?** Define whether `path` is a CUE path,
   dot path, JSON pointer, or another grammar. Resolve nested fields, root
   scalars, hidden fields, aliases, list indexing, and the rule that the first
   slice rejects non-scalar and list-valued leaves.

   **Resolved:** `path` is an absolute RFC 6901 JSON Pointer into the concrete
   exported catalog resource. It traverses exact, case-sensitive object fields
   with standard escaping. The first slice rejects the empty root, arrays and
   list indexes, objects or arrays as final values, wildcards, filters, relative
   paths, and CUE-only aliases/definitions/optional/hidden structure. Concrete
   string, number, boolean, and explicit `null` leaves are allowed; missing and
   `null` remain distinct and CUE validates whether the input accepts `null`.

8. **What is the input completeness rule?** Decide whether every declared
   input must be bound, or only inputs referenced by `desired`, `checks`, or
   another phase. Define nested input-slot naming, duplicate bindings,
   unbound inputs, and the validation/error timing for incomplete models.

   **Resolved:** inputs are flat, visible top-level scalar slots, and every
   declared input has exactly one same-named binding; extra bindings, optional
   slots, and nested slots are rejected. Names are exact labels, not paths, and
   defaults do not make bindings optional. Keyed CUE declarations unify
   normally, with conflicts reported by CUE rather than AST duplicate checks.
   Static shape/completeness failures occur before any run-set member executes;
   resolved values undergo final concrete CUE validation before the consumer
   run or mu plan begins.

9. **How are raw CUE templates retained?** Model discovery currently validates
   and decodes a model, then discards the original `cue.Value`. Define a
   `ModelTemplate` or registry seam that retains the raw value, decoded
   metadata, module root, and compatible CUE context for later unification.

   **Resolved:** discovery returns an in-memory
   `systemmodel.ModelTemplate` containing the loaded template `cue.Value`,
   concrete/canonical identity, load directory, owning PUDL root, definition
   origin, and decoded bindings. It validates incomplete CUE and decodes only
   preflight metadata. `Elaborate` builds inputs in the same CUE context,
   immutably unifies and concretely validates a fresh value, then produces the
   existing runtime `SystemModel`. Neither template values nor resolved values
   are persisted or cached across runs.

10. **Where does binding metadata live?** Model instances are serialized into
    the catalog from the decoded system model. Decide whether `inputs` and
    `bindings` remain template-only, or whether catalog serialization uses an
    explicit redacted projection so authoring metadata does not leak into
    model-instance resources or mu inputs.

    **Resolved:** `inputs` and `bindings` are template-only and are absent from
    runtime `SystemModel`. Catalog model-instance serialization and mu rendering
    consume only that explicit runtime projection. Referenced non-secret values
    may appear naturally in executable desired/config/check fields, while
    binding provenance belongs in reports and dependency edges in their
    relation. Sentinel contract tests must prove authoring namespaces reach
    neither catalog model JSON nor mu input.

11. **How is the secret boundary enforced?**

    **Resolved contract:** plain and sealed bindings are distinct,
    non-coercible channels. `@pudl(binding=plain)` permits a schema field and
    consumer slot to participate in catalog-scalar CUE elaboration.
    `@pudl(binding=sealed)` identifies a phase-owned sealed input or a
    converge-owned output declaration. Unannotated fields fail closed; inherited annotations are
    honored; conflicting classifications are errors. Sealed bindings transport
    only a provider reference and execution metadata, never a value. The
    single-model path lowers fully
    to mu `sealed_inputs`, `sealed_input_modes`, `sealed_outputs`,
    `sealed_output_modes`, `resolve_secret`, `store_secret`, pith `secret/get`
    taint, and `secrets.writable_refs`; cross-model exact-plan validation awaits
    mu action claims in `pudl-olm`. PUDL never resolves or stores secret
    values. Provider references pass through generated mu configuration and
    mu's action key/provider calls as required; PUDL persists only their scheme
    and a fingerprint, and no resolved value enters CUE, catalog rows, reports,
    manifests, caches, or logs.

    Sealed inputs live directly on the `populate` or `converge` arm that
    consumes them; sealed outputs are converge-only in v1. Their map key is the
    mu-visible name, with no generic port/attachment layer. An input declares
    exactly one direct `ref` or a cross-model `source: {model, output}`. A
    producer output exclusively owns its provider reference and store mode; a
    source-bound consumer owns only its local name and `env`/`file` delivery
    mode and cannot override the reference. A direct-ref consumer owns its
    reference. Workspace/run-set policy exclusively owns
    `secrets.writable_refs`.

    PUDL-generated mu targets use strict explicit sealed routing. Phase-level
    declarations make names available to plugin planning but never implicitly
    grant them to every action. Each consuming action explicitly claims its
    sealed inputs, and every sealed output has exactly one explicitly claiming
    producer action. Planning rejects implicit fan-out, unused declarations,
    undeclared claims, and ambiguous outputs. Mu's existing sealed execution
    and provider machinery remain authoritative; v1 adds the strict routing
    policy needed for least privilege.

#### Dependency graph and provenance

12. **What is the canonical edge direction?** The design diagram uses
    producer-to-consumer edges, while existing `model_depends_on` facts use
    consumer-to-producer semantics. Choose the persisted representation and
    document the conversion used for topological execution order.

    **Resolved:** consumer to producer remains canonical everywhere authored,
    reported, queried, or persisted: `model_depends_on(from: consumer, to:
    producer)`. Plain bindings and cross-model sealed sources share this
    meaning; direct provider refs create no model edge. The scheduler reverses
    canonical edges only to compute producer-first execution order. Diagrams
    may show execution flow but do not change relation semantics.

13. **Are binding edges persisted?** A binding implies a data dependency, but
    it is not yet clear whether that edge is emitted as a dependency fact,
    shown by `pudl model deps`, or kept as an execution-only edge. If persisted,
    define its provenance, reconciliation with explicit `depends_on`, and
    behavior when the binding changes or disappears.

    **Resolved:** persist authoritative binding-derived edges in the existing
    `model_depends_on(from, to)` relation under `binding:<consumer>`. Aggregate
    plain bindings and cross-model sealed sources by producer and atomically
    reconcile the consumer's complete wanted set after successful template
    validation. Direct provider refs create no edge. Declared, heuristic, and
    binding sources reconcile independently; coincident edges are coalesced for
    queries/display with combined provenance. Removing the last binding
    bitemporally invalidates only the binding-sourced fact. Per-input and
    per-run evidence stays in reports rather than generating fact churn.

14. **What provenance is reported?** Every resolved value should identify the
    pinned snapshot ID, workspace, producer run, observation age, and reuse
    decision (current-run versus prior observation). Define the durable report
    shape and error detail before implementing the selector.

    **Resolved:** add a versioned `RunSetReport` keyed by run-set ID for the
    plan digest, approval, graph, member ordering, and terminal member states;
    link versioned per-model `RunReport`s by run-set ID. Plain evidence records
    the exact authorized scalar/type/digest plus complete producer snapshot,
    workspace, selector, age, and reuse provenance. Sealed evidence records
    phases, names, modes, action routing, provider scheme/reference fingerprint,
    policy match, producer provenance, and lifecycle status, but never a secret
    value or secret-value hash. Typed mutation receipts preserve completed
    partial state, and structured errors are redacted. Reports use explicit
    structs and a schema version rather than untyped maps.

#### Acceptance and compatibility

15. **What failure and race cases are contractual?** Acceptance coverage must
    include workspace isolation, no snapshot, current-run producer failure
    without fallback, pinned snapshot selection, invalid paths, list/non-leaf
    rejection, duplicate and unbound inputs, and producer/consumer cycles.

    **Transaction boundary accepted:** reserve run-set/member/snapshot IDs
    atomically; select reused snapshots and copy binding evidence in one short
    read transaction; commit each current-run observation as one atomic step;
    close every transaction before invoking mu. Persist plan/approval and each
    execution receipt in short write transactions. Approval reconstruction uses
    pinned evidence plus current model/plugin/policy fingerprints and rejects
    any digest change. Pending approvals retain their snapshots. Missing
    evidence becomes stale, and a crash after possible mutation but before its
    receipt becomes `needs-verification` with no automatic retry or resume.

    **Resolved release gate:** v1 requires deterministic coverage across CUE
    classification, plain selection, sealed declaration/provider/mode policy,
    strict mu action routing, graph ordering/reconciliation, exact approval,
    fail-fast and crash recovery, concurrency/retention, versioned reporting,
    and compatibility. The end-to-end path runs real mu planning/execution
    against a fake provider implementing `resolve_secret` and `store_secret`.
    Live credentialed providers are optional smokes; the fake-provider matrix
    and complete PUDL/mu gates are release-blocking.

16. **Which existing documents are superseded?**
    [`docs/cross-model-dependencies.md`](cross-model-dependencies.md) currently
    says value passing is deferred and that PUDL does not re-run downstream
    models. The wiring design should explicitly mark which statements remain
    true and which are replaced by the new bounded value-flow contract.

    **Resolved:** the dependency relation, rules, reconciliation, and explicit
    `depends_on` semantics remain authoritative. The older claims that all value
    passing and downstream orchestration are out of scope are superseded.
    Standalone runs still never start producers implicitly; explicit `run-set`
    coordinates exactly the named models while mu executes each member graph.
    The historical Swamp roadmap retains its original deferred boundary with a
    successor note. `VISION.md` now distinguishes PUDL coordination from mu
    execution. User guides, CLI reference, embedded skills, and generated help
    update with implementation rather than documenting unavailable commands as
    shipped.

These questions are a design checkpoint, not additional binding configuration.
The intended user-facing API remains convention-driven; the extra detail is
needed to make selection, failure, provenance, and orchestration deterministic
under the hood.

### Richer Catalog
- Schema coverage reports (what percentage of data matches specific schemas)
- Catalog-driven code generation and documentation
- Cross-source correlation (linking AWS resources to Kubernetes resources)
- Temporal tracking (same resource across imports via `resource_id` + `version`)

### Deeper Mu Integration
- Richer action types beyond field-level drift (create, delete, reconcile)

### More Type Patterns
- Azure, GCP, Terraform state files
- Docker Compose, Helm values, CI/CD pipeline configs
- User-defined pattern registration

### Analytics
- ~~`pudl summary` / `pudl stats` for aggregate views~~ → Done: `pudl facts stats --group-by`
- `pudl diff` to compare two versions of the same resource
- Basic outlier detection across instances of a schema
- DuckDB/Parquet integration for analytical queries on large datasets

### UI Improvements
- Dashboard/reporting for catalog and drift state
- Interactive TUI for browsing catalog entries and schemas

## Core Packages

| Package | Path | Responsibility |
|---------|------|----------------|
| `importer` | `internal/importer/` | Import pipeline, format detection, streaming, and NDJSON collections |
| `inference` | `internal/inference/` | Schema inference (heuristics + CUE unification) |
| `identity` | `internal/identity/` | Resource identity extraction and computation |
| `idgen` | `internal/idgen/` | Content IDs, SHA256, proquint encoding |
| `database` | `internal/database/` | SQLite catalog CRUD and queries |
| `validator` | `internal/validator/` | CUE validation |
| `systemmodel` | `internal/systemmodel/` | `#SystemModel` schema and model loading |
| `mubridge` | `internal/mubridge/` | Typed envelope, manifest, and observe-snapshot ingestion |
| `workspace` | `internal/workspace/` | Per-repo workspace discovery, context resolution |
| `schemaname` | `internal/schemaname/` | Schema name normalization |
| `schemagen` | `internal/schemagen/` | Schema generation from data |
| `typepattern` | `internal/typepattern/` | Pluggable type detection patterns |
| `streaming` | `internal/streaming/` | CDC chunkers, format processors |
| `config` | `internal/config/` | YAML configuration |
| `init` | `internal/init/` | Workspace initialization |
| `git` | `internal/git/` | Git operations on schema repo |
| `lister` | `internal/lister/` | List/query with filters |
| `doctor` | `internal/doctor/` | Health checks, directory validation |
| `schema` | `internal/schema/` | Schema operations |
| `repo` | `internal/repo/` | Repo init, skill installation |
| `skills` | `internal/skills/` | Embedded Claude skill files |
| `ui` | `internal/ui/` | Output formatting |
| `errors` | `internal/errors/` | Typed error codes |
| `cmd` | `cmd/` | CLI command definitions (Cobra) |

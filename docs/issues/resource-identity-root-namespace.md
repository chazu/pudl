# Decouple resource identity from the assigned schema (root-of-family namespacing)

**Priority:** High
**Status:** Implemented (2026-06-10) — see `implog/2026_06_10_resource_identity_root_namespace.md`
**Date:** 2026-06-10

## Summary

`resource_id` is currently namespaced by the **assigned (leaf) schema name**:

```
resource_id = SHA256( normalize(assigned_schema) + "\x00" + identityComponent )
```
(`internal/identity/resource_id.go:15`)

This couples a resource's *identity* to its *classification*. Two problems follow:

1. **Reinference silently re-identifies resources.** `pudl schema reinfer`
   recomputes `resource_id` from `entry.Schema` (`cmd/schema_reinfer.go:399`). Any
   time reinference changes a schema assignment, the resource's `resource_id`
   moves, its version sequence resets, and facts referencing the old id are
   orphaned. This is a latent correctness bug independent of any use case.

2. **Policy/specialization schemas fragment identity.** Because inference assigns
   the most-specific schema that unifies (policy schemas included — only
   `catchall` is special-cased; `internal/inference/heuristics.go:175`), a
   resource that crosses a compliance/specialization boundary (e.g.
   `GarnerGitRepository` → `CompliantGarnerGitRepository`) gets a **different
   `resource_id`** and reads as a brand-new resource, losing version/drift
   history.

The original intent of identity fields was to give a resource a *stable* identity
so JSON from different sources could be linked and deduplicated. The schema-name
coupling defeats that.

## Decision

**Namespace `resource_id` by the root of the schema's inheritance family**, not the
assigned leaf schema:

```
resource_id = SHA256( normalize(identity_root) + "\x00" + identityComponent )
```

- `identity_root` = the topmost ancestor of the assigned schema in the
  inheritance graph (walk `base_schema` to the top).
- `identityComponent` is unchanged: canonical identity JSON when identity fields
  are present, else the content hash (catchall fallback, unchanged).

Why root-of-family:

- The root is **invariant** under the two operations that currently break
  identity — reinference moves the *leaf*, never the *root*; a policy schema
  written as `#Compliant: #Base & {…}` shares the base's family. So compliant /
  non-compliant / reinferred versions of one resource collapse to one stable id.
- It is **derived purely from the subsumption graph** — no new declared metadata
  field (we explicitly rejected an `identity_namespace` field in favor of this).
- It **preserves cross-kind collision safety**: different families → different
  roots → cannot collide, even with weak identity fields.
- When identity fields are globally unique (URLs, ARNs, full paths) the root
  namespace is invisible — it only does work in the weak-id case, where you want
  a backstop.

### The family invariant this assumes

For dedup to be correct, every schema a single resource can be classified under
must extract the **same identity values from the same fields**. Therefore:

> `identity_fields` must be identical across an inheritance family. They are
> declared at the family root and inherited unchanged; descendants may *tighten
> constraints* on those fields but must not change the field set or the values
> extracted.

When families are built with CUE unification (`#Child: #Base & {…}`) CUE enforces
this for free (divergent `identity_fields` lists fail to unify). The risk is
`base_schema` declared as a bare string reference without CUE inheritance — that
is what the doctor check below backstops.

## Scope

Refactor only. Authoring the `GitRepository → GitlabRepository →
GarnerGitRepository (+ Compliant policy)` family is the motivating use case but is
**separate, follow-up work** once this lands.

## Implementation

Sequenced so each step builds and tests green (`CGO_ENABLED=0 go test ./...`).

### 1. Family-root resolver (`internal/inference/graph.go`)

Add:

```go
// IdentityRoot returns the topmost ancestor of schema in the inheritance
// family (the schema used to namespace resource identity). Returns schema
// itself if it has no parent or is unknown to the graph.
func (g *InheritanceGraph) IdentityRoot(schema string) string
```

Implementation: last element of `GetCascadeChain(schema)` (already cycle-safe,
capped at 100). Returns `schema` for roots and unknown schemas.

Tests (`graph_test.go`): leaf→root, mid→root, root→itself, unknown→itself,
cycle-safe.

### 2. Identity hash semantics (`internal/identity/resource_id.go`)

- No logic change beyond meaning: the first argument is now the **identity
  namespace (family root)**, not the leaf schema. Rename the parameter and update
  the doc comment. `schemaname.Normalize` stays.
- `internal/identity` remains pure (no `inference` import). Callers resolve the
  root and pass it in.

Tests (`resource_id_test.go`): two records in the same family but assigned
different leaves now produce the **same** `resource_id`; catchall (no identity
fields) unchanged.

### 3. Call sites — resolve root before hashing

All resolve `root := <graph>.IdentityRoot(assignedSchema)` then pass `root`:

- `internal/importer/enhanced_importer.go:165, 320, 458` — `e.inferrer.GetInheritanceGraph()` is in scope.
- `cmd/schema_reinfer.go:399` — `inferrer.GetInheritanceGraph()`.
- `internal/mubridge/ingest.go:192, 272` — **no graph in scope.** Thread
  `*inference.InheritanceGraph` (or the inferrer) through `IngestObserveResults` →
  `createObserveSnapshot` / `ingestObserveRecord`, and pass it from the cmd
  caller. (The `pudl/mu.#…` schemas are roots today, so behavior is unchanged in
  practice, but thread it for correctness against future `base_schema` additions.)

### 4. Family-consistency doctor check (`internal/doctor/checks.go`)

Add `CheckIdentityFieldConsistency() *CheckResult`, mirroring the existing
`CheckPudlNamespaceSchemas` pattern:

- For each schema with a `base_schema`, compare its `identity_fields` to its
  base's (walk the chain). Flag any divergence as a `warning` with a fix pointing
  at the family-root invariant. `ok` when all families are consistent.
- Register in `cmd/doctor.go` ("Identity Fields") and add a test in
  `internal/doctor/checks_test.go`.

### 5. Migration — explicit, idempotent recompute (`cmd` + `internal/database`)

This change alters **every** existing `resource_id`, so a one-time recompute is
required. Deliver as an explicit command (e.g. extend `pudl migrate identity` with
`--recompute`, or add `pudl migrate resource-ids`). **Not** auto-on-open — it
changes existing values and must be deliberate. Recompute is a pure function of
`(identity_root, identity_values)`, so the command is naturally idempotent.

Behavior:

1. Load **all** entries (not just NULL `resource_id`).
2. Per entry: read the data file, get the assigned schema's `identity_fields`,
   extract identity values, resolve `identity_root` via the graph, recompute
   `resource_id` and `identity_json`.
3. **Re-sequence versions.** Group entries by the *new* `resource_id`; within each
   group order by a stable time key (import/created timestamp, tie-break by id)
   and assign `version` `1..N`. This is the highest-risk sub-task because
   regrouping under family roots can **merge** previously-distinct `resource_id`s
   (same family, different leaves) into one resource that now needs one coherent
   monotonic history. Confirm the ordering timestamp field on `CatalogEntry`
   during implementation.
4. `--dry-run` prints `old → new` `resource_id` and version changes.

Test on a temp catalog (`os.MkdirTemp` + `NewCatalogDB`): seed entries across a
family with old-formula ids, run recompute, assert ids collapse to the family
root and versions resequence monotonically; assert idempotency (second run is a
no-op).

### 6. Docs + implog

- `docs/schema-authoring.md` — new "Resource identity & schema families"
  subsection: the family invariant, root-namespaced `resource_id`, stability
  under reinference/policy refinement, and the doctor check.
- `docs/plan.md` — update the "Content-based identity / Resource identity" bullets
  to reflect root-of-family namespacing.
- `implog/2026_06_10_resource_identity_root_namespace.md` on completion.

## Risks & caveats

- **Breaking by design.** All `resource_id`s change. Communicated via the explicit
  migrate command + docs. Acceptable per decision (nothing external stores
  `resource_id` values).
- **User-authored facts/datalog rules that hardcode a `resource_id` value** will
  not auto-migrate. Note in the migration output/docs.
- **Version resequencing** is the part to test hardest — isolate it and cover the
  merge case explicitly.
- **Abstract roots:** if a family root declares no identity fields but a
  descendant introduces them, "topmost ancestor" still namespaces correctly as
  long as the family invariant holds; the doctor check enforces consistency.

## Sequencing

1. `IdentityRoot` helper + tests
2. `ComputeResourceID` semantics + all call sites (incl. mubridge threading)
3. Doctor consistency check + test
4. Migration command + version resequencing + test
5. Docs + implog

Each step independently builds and passes `CGO_ENABLED=0 go test ./...`.

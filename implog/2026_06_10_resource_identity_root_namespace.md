# Decouple resource identity from the assigned schema (root-of-family namespacing)

Date: 2026-06-10

## Goal

Stop namespacing `resource_id` by the *assigned (leaf) schema* and namespace it
by the *root of the schema's inheritance family* instead. The previous coupling
caused two problems:

1. `pudl schema reinfer` recomputed `resource_id` from `entry.Schema`, so any
   reclassification silently changed a resource's identity (version reset,
   orphaned fact references).
2. Specialization/policy schemas (assigned by inference, most-specific-first)
   fragmented identity: a resource crossing a boundary like
   `GarnerGitRepository` → `CompliantGarnerGitRepository` got a different
   `resource_id` and read as a brand-new resource.

The family root is invariant under both reinference (moves the leaf) and policy
refinement (a `#Compliant: #Base & {...}` shares the base's family), so identity
stays stable while classification changes.

```
resource_id = SHA256( normalize(family_root) + "\x00" + identity_component )
```

## Changes

### internal/inference
- **`graph.go`** — `(*InheritanceGraph).IdentityRoot(schema) string`: topmost
  ancestor of `schema` (last element of `GetCascadeChain`); returns `schema`
  itself when it is a root or unknown to the graph.
- **`graph_test.go`** — `TestIdentityRoot` (leaf/mid → root, root → itself,
  unknown → itself).

### internal/identity
- **`resource_id.go`** — `ComputeResourceID`'s first parameter is now the
  *identity namespace* (the family root), not the leaf schema. Hashing logic
  (normalize, canonical JSON / content-hash fallback) is unchanged; callers
  resolve the root and pass it in. `internal/identity` stays pure (no
  `inference` dependency).

### Call sites (resolve family root before hashing)
- **`internal/importer/enhanced_importer.go`** — new `identityNamespace(schema)`
  helper (via `e.inferrer.GetInheritanceGraph().IdentityRoot`); used at the
  single-entry, collection, and collection-item `ComputeResourceID` sites.
- **`cmd/schema_reinfer.go`** — `recomputeEntryIdentity` resolves the root via
  `inferrer.GetInheritanceGraph().IdentityRoot(entry.Schema)`.
- **`internal/mubridge/ingest.go`** — threads `*inference.InheritanceGraph`
  through `IngestObserveResults` → `ingestObserveRecord` (routed records can
  resolve to non-root schemas); nil-safe `identityNamespace` helper falls back to
  the schema itself. The snapshot entry uses a literal root schema. Caller
  `cmd/ingest_observe.go` builds the graph from the inferrer.

### internal/doctor (family identity invariant)
- **`checks.go`** — `CheckIdentityFieldConsistency`: for each schema with a base,
  warns when its `identity_fields` differ from its base's (CUE `&` unification
  enforces consistency for free; this backstops bare `base_schema` references).
  Helper `sameStringSet`. Registered in `cmd/doctor.go` as "Identity Fields".
- **`checks_test.go`** — `TestCheckIdentityFieldConsistency` (consistent → ok,
  divergent → warning) using a temp `$HOME/.pudl/schema` CUE module.

### Migration (cmd)
- **`cmd/identity_migrate.go`** — `pudl migrate identity` gains `--recompute`:
  recomputes `resource_id`/`identity_json` for **all** entries using family-root
  namespacing, then re-sequences `version` per (new) `resource_id` ordered by
  import time (tie-broken by created time, then id). Idempotent; honors
  `--dry-run` (prints `old → new` + version). Default mode still backfills only
  NULL `resource_id` entries, now also root-namespaced. New helpers
  `migrateEntryIdentity` (reads the data file only when the schema has identity
  fields), `assignVersions`, `shortRID`, `versionSuffix`.
- **`cmd/identity_migrate_test.go`** — `TestAssignVersions` (merge ordering +
  idempotency), `TestMigrateEntryIdentity` (leaf → root namespace, catchall uses
  content hash without reading the file).

## Public API

- `inference.(*InheritanceGraph).IdentityRoot(schema string) string`
- `identity.ComputeResourceID(identityNamespace string, identityValues map[string]interface{}, contentHash string) string`
  (signature unchanged; first arg semantics now "identity namespace = family root")
- `mubridge.IngestObserveResults(db, reader, origin, dataDir string, graph *inference.InheritanceGraph) (int, error)`
  (added `graph` parameter)
- `doctor.CheckIdentityFieldConsistency() *CheckResult`
- CLI: `pudl migrate identity --recompute [--dry-run]`

## Verification

`CGO_ENABLED=0 go build ./...` and `CGO_ENABLED=0 go test ./...` both green.

## Notes / caveats

- This change alters every `resource_id`; existing stores must run
  `pudl migrate identity --recompute` once. User-authored facts/datalog rules
  that hardcode a `resource_id` value will not auto-migrate.
- Scope was the identity refactor only. Authoring the motivating
  `GitRepository → GitlabRepository → GarnerGitRepository (+ Compliant policy)`
  family is separate follow-up work. See
  `docs/issues/resource-identity-root-namespace.md`.

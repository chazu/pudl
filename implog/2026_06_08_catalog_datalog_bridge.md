# Catalog-as-datalog bridge (Phase 2)

Date: 2026-06-08

## Goal

Let Datalog rules query the catalog alongside facts through the same
`Store.Query` API: expose `catalog_entries` as a built-in `catalog_entry`
relation usable as a rule body atom, so rules can join facts against catalog
data (e.g. join an asserted fact to the catalog entry it refers to).

## Mechanism

The SQL compiler already supports native-column tables via
`CompileOptions.TableOverrides`: for an overridden relation it accesses
`alias."col"` directly (no `json_extract`), and skips both the `relation = ?`
filter and the temporal-scope filters. No JSON re-shaping is needed — pointing
`catalog_entry` at a SQL view makes the compiler read native columns. The
recursive evaluator already threads `TableOverrides` for `_delta_` temp tables;
the catalog override merges in.

A curated SQL view (not a raw table override) backs the relation so the Datalog
interface does not depend on the physical catalog schema.

## Changes

### internal/database
- **New `builtin_relations.go`** — owns the reserved-name contract:
  - `CatalogEntryRelation = "catalog_entry"`, `CatalogEntryView = "catalog_entry_edb"`
  - `reservedRelations` set; `IsReservedRelation(rel) bool`; `ReservedRelations() []string`
- **New `catalog_entry_view.go`** — `ensureCatalogEntryView()` drops+recreates
  the `catalog_entry_edb` view (idempotent; always matches source). Curated
  columns: `id, schema, origin, format, status, entry_type, definition, method,
  run_id, resource_id, content_hash, version, collection_id, collection_type,
  item_id`. Internal/volatile columns (stored_path, metadata_path,
  identity_json, tags, timestamps, size/record counts) are excluded.
- **`catalog.go`** — call `ensureCatalogEntryView()` at the end of
  `createTables()`, after all column migrations (the view references
  migration-added columns).
- **`facts.go`** — `AddFact` rejects reserved relations with a clear error,
  preventing user facts from silently shadowing the built-in relation.

### internal/datalog
- **New `builtin_edb.go`** — `builtinEDBTables = {catalog_entry: catalog_entry_edb}`
  (referencing the database constants); `withBuiltinEDB(overrides)` merges the
  built-in mappings into a caller's override map (caller wins; no collision
  since built-ins are never derived).
- **`sql_eval.go`** — `Query` compiles with `CompileWithOptions(..., builtinEDBTables)`.
- **`recursive.go`** — `seedBase` compiles base rules with `builtinEDBTables`;
  `fixpointLoop` merges them via `withBuiltinEDB(overrides)` so recursive rule
  bodies can reference `catalog_entry`.

No public API change: `factstore.Store.Query` already routes through
`datalog.Evaluate`, so consumers can use `catalog_entry` body atoms immediately.

## Tests

- `internal/datalog/catalog_edb_test.go`:
  - rule over `catalog_entry` alone (SQL path)
  - `catalog_entry` joined against a fact relation on a shared variable
    (cross-source join — native columns × JSON args)
  - recursive rule set with a catalog base rule (seedBase override) and
    `catalog_entry` in the recursive body (fixpointLoop override)
  - query under a temporal scope (catalog atom not temporally filtered)
  - sync test: every `builtinEDBTables` key is `database.IsReservedRelation`
- `internal/database/builtin_relations_test.go`:
  - `AddFact` rejects the reserved relation; allows normal relations
  - `catalog_entry_edb` view projects catalog rows
- `CGO_ENABLED=0 go test ./...` green (26 packages), `go vet` clean.

## Notes / limitations

- `catalog_entry` works as a rule **body atom**, not as a direct query target.
  A bare query for `catalog_entry` with no rule hits the facts-table fallback and
  returns nothing. Direct-query support (a `fallbackEDB` branch over the view) is
  a future add if needed.
- Datalog arg names map to view column names. Nullable columns bind to nil and
  joins on NULL never match (expected). Numeric view columns vs string CLI
  constraints rely on SQLite type affinity.
- `catalog_entry` is a reserved relation name (enforced at `AddFact`).

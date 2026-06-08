# Catalog direct-access guard, ListCatalog, and documentation

Date: 2026-06-08

Follow-on to the public API extraction (Phase 1) and catalog-as-datalog bridge
(Phase 2). Two small features plus a documentation sweep.

## Features

### Join-only guard for built-in EDB relations
`internal/datalog/query.go` — `Evaluate` now returns an explicit error when a
built-in EDB relation (e.g. `catalog_entry`) is queried directly with no
producing rule, instead of silently falling through to the facts table and
returning nothing. Helper `relationHasAnyRule`. Test:
`TestCatalogEDBDirectQueryRejected`.

### Store.ListCatalog (typed catalog access)
`pkg/factstore/catalog.go` — `(*Store) ListCatalog(filter CatalogFilter, query
CatalogQuery) (*CatalogResult, error)`, wrapping `database.QueryEntries` on the
Store's existing DB handle (no second open). Plain-data aliases added:
`CatalogEntry`, `CatalogFilter`, `CatalogQuery`, `CatalogResult`. Read-only
(catalog writes remain the import pipeline's job). White-box test
`TestListCatalog`.

## Documentation

- **New `docs/library-api.md`** — the public Go API (`pkg/factstore`,
  `pkg/eval`): signatures, `QueryOptions`, resolution helpers, a full usage
  example, and the `catalog_entry` join-only note. Linked from `docs/README.md`
  and `docs/architecture.md`.
- **`docs/datalog.md`** — rewrote the EDB "Catalog" section: now documents the
  view-backed `catalog_entry` relation, its full field set, a join example, and
  the join-only/reserved semantics. Removed the stale `CatalogEDB`/`MultiEDB`
  description.
- **`docs/architecture.md`** — added `datalog` and a "Public API (`pkg/`)"
  subsection (factstore, eval) to the package table.
- **`docs/facts.md`** — noted `catalog_entry` is reserved/join-only.
- **`CLAUDE.md`** — refreshed the Datalog & Fact Store section: dropped the
  deleted `eval.go`, added the `Evaluate` orchestrator, `catalog_entry` built-in,
  and the public API pointer.
- **`cmd/guide.go`** (`pudl guide datalog`) — fixed a pre-existing bug: the rule
  and SQL examples used an old positional-list syntax (`head: [...]`,
  `args->>'$[0]'`) that does not match the real named-arg CUE format. Corrected
  to `head: {rel, args}` / `json_extract(args, '$.key')`, and added a "Catalog as
  a relation" section.

`CGO_ENABLED=0 go test ./...` green (26 packages).

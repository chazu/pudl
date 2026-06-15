# Component/Schema boundary (D1 of git-repository decomposed-resources)

**Date:** 2026-06-15
**Design doc:** `docs/issues/git-repository-decomposed-resources.md` (decision D1)

## Summary

Made the `_pudl` metadata block the explicit boundary between a **schema** (a
tracked resource type, eligible for inference and identity) and a **component**
(a reusable shape meant to be embedded in a schema, inert to inference).

Previously `createModuleFromInstance` registered *every* `#`-prefixed definition
as a candidate schema. Components like `pudl/aws.#Tag`, `pudl/infra.#ServiceBinding`,
`pudl/component.#ComponentKind`, and `pudl/catalog.#CatalogEntry` were registered
as phantom schemas — inert only by accident of scoring (the inference boosts in
`internal/inference/heuristics.go` gate on non-empty identity/tracked fields), but
still visible in schema listings and reinference candidate sets.

Now a `#`-definition is registered only if it carries a `_pudl` block. The single
exception is **list-type definitions** (e.g. `#CatchAllCollection: [...]`), which
legitimately carry no `_pudl` because CUE arrays have no fields; these remain
registered so collection detection (`IsListType`) keeps working.

This is the load-bearing engine change of the git-repository design doc, done
first and independently per the doc's sequencing. The git CUE schemas themselves
(base `#GitRepository` + `#GitHubRepository`/`#GitLabRepository` specializations,
with inline `#GitRemote`/`#GitBranch` components) are a separate follow-up.

## Change

- `internal/validator/cue_loader.go` — `createModuleFromInstance`: reordered the
  per-definition loop to detect `_pudl` presence *before* registration, and
  `continue` (skip registration of name + metadata) when a definition has no
  `_pudl` block and is not a list type. No public API surface changed; the filter
  is internal to module loading.

## Behavior impact

- Components no longer appear as schemas in `GetAllSchemas` / `GetAllMetadata`,
  `pudl` schema listings, or inference candidate selection.
- CUE shape resolution is unaffected: a schema referencing a component as a field
  type (`remotes: [...#GitRemote]`) still validates, because the reference is
  resolved inside the compiled `cue.Value` — registration is pudl bookkeeping, not
  CUE resolution.
- `base_schema` chains are unaffected (base schemas always carry `_pudl`).

## Public API

None changed. Affected symbols are all unexported or unchanged in signature:
- `createModuleFromInstance` (unexported) — behavior change only.
- `GetAllSchemas` / `GetAllMetadata` (unchanged signatures) — now return fewer,
  correct entries.

## Tests

- `internal/validator/cue_loader_test.go` — new `TestComponentSchemaBoundary`:
  scaffolds a package with a `_pudl` schema, a `_pudl`-less component, and a
  `_pudl`-less list-type schema; asserts the schema and list-type are registered
  while the component is not (in both `schemas` and `metadata` maps).
- Full suite green (`CGO_ENABLED=0 go test ./...`), including importer / inference
  / integration / system tests that load and infer against the bootstrap schemas
  end-to-end.

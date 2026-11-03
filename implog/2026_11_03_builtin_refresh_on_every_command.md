# Built-in schemas refresh on every command

## Problem

`pudl graph vocab` in this repo's own workspace reported 17 problems: 16
`graph_edge` rules in `gcp_graph.cue` deriving "undeclared" edge types, and
`infra.organization` with no display info. Every one of those edge types is
declared in the built-in `pudl/rules/graph_vocabulary.cue`. The workspace's
built-ins came from an older pudl (no `.bootstrap-digest` stamp) and
predated that file and the removal of `infra.#Organization`.

Built-ins were refreshed only by `EnhancedImporter.ensureBasicSchemas`, which
runs on import. Read commands (`graph vocab`, `doctor`, `ui`, `query`,
`check`) loaded the stale copies, and the vocab pointed at the pack's rule
file.

## What changed

- `internal/importer/bootstrap_stamp.go`: `RefreshBootstrapSchemas`.
- `internal/importer/cue_schemas.go`: `ensureBasicSchemas` uses it.
- `cmd/builtin_refresh.go`: `refreshBuiltinSchemas(errOut)`, called from the
  root `PersistentPreRunE` once the workspace policy is resolved. It refreshes
  the invocation's primary schema path only (the repo workspace, or `~/.pudl`
  in global mode). On a refresh it prints `Updated pudl's built-in schemas in
  <dir>` on stderr. A failed refresh warns and does not fail the command.
- `docs/workspace.md`: built-ins refresh on the first command, not the first
  import.

## Public API

```go
// package importer
// RefreshBootstrapSchemas rewrites the built-in schemas under schemaPath when
// they are not this pudl's. An uninitialized schema repository (no
// cue.mod/module.cue) is left alone. Reports whether it rewrote them.
func RefreshBootstrapSchemas(schemaPath string) (bool, error)
```

## Tests

- `internal/importer/bootstrap_refresh_test.go`: uninitialized repository
  untouched; current built-ins not rewritten; stale built-ins restored and
  stamped.
- `cmd/builtin_refresh_test.go`: stale workspace + GCP example →
  `graph vocab` refreshes once, reports no problems.

## Workspace fix (this repo)

`pudl example install gcp` upgraded the GCP pack's 8 changed files (they had no
`graph: display` blocks). Running the new pudl refreshed `.pudl/schema/pudl/`
(17 tracked files changed, retired `brick/brick.cue` removed). `pudl graph
vocab`: 21 resource types, 36 edge types, no problems.

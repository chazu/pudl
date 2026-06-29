# Delete the World A definition/socket subsystem (Cluster A step 3)

**Date:** 2026-06-26

## Summary

Final step of the Cluster A plan (docs/vestige-sweep.md §6): delete the
pre-`pudl run` "definitions + sockets" subsystem and its standalone
drift/status/export path, now that the salvaged capabilities (model
list/show/validate, run-status persistence) live on the SystemModel spine.

Rationale (recorded in §6): the SystemModel design deliberately relocated the
concerns World A addressed — ordering → the mu DAG, relating → Datalog,
capabilities → mu plugins. World A was the pre-decision model, superseded by
construction.

## Deleted

- **Commands:** `pudl definition list/show/validate/graph`; `pudl drift
  check/report`; `pudl mu export-actions`; `pudl repo validate`.
- **Packages/files:** `internal/definition/` (Discoverer, SocketBindings,
  dependency graph, BRICK `interface_checker`, validator); `internal/drift/`
  (the standalone `Checker` + `.drift/` ReportStore — the run loop never imported
  it, so no diff-type untangle was needed); `internal/mubridge/export.go`
  (`ExportMuConfig` + World-A-only types); `internal/database/catalog_artifacts.go`
  (`GetLatestArtifact*`, the dead `entry_type='artifact'` reads).

## Survivors reframed

- **`cmd/status.go`** — reduced to a pure catalog-status reader (dropped the
  `internal/drift` ReportStore diff-count enrichment + `DriftResult` detail). It's
  the read side of run-status persistence (step 2); now surfaces model verdicts
  on `//models/<name>` rows.
- **`cmd/repo.go`** — keeps `repo init`; dropped `repo validate` + the
  `internal/definition` import.
- **`cmd/mu.go`** — dropped the `export-actions` subcommand wiring + help.
- **`cmd/guide.go` / `cmd/prime.go`** — `definitions` guide topic → `models`
  (covering `pudl model …` + `pudl run`); dropped the stale standalone `drift`
  topic; the `mu` guide lost its dead `export-actions`/`drift check` workflow and a
  leftover "SHARED PITH VM" section (pith was removed in Cluster B).
- **`internal/database/catalog_observe_test.go`** — re-homed the shared test
  helpers the deleted `catalog_artifacts_test.go` provided, dropping the dead
  `artifact` naming/type (`setupTestCatalog`, `addTestManifestEntry`).

## Not done (noted)

- `CatalogEntry.Method` column — a leftover of the definition→method→artifact
  model (nullable, harmless); removing it touches many SELECT/Scan sites — deferred.
- `resource_type` `artifact` / `artifact.image` + embedded `pudl/artifact/*.cue`
  — a separate axis from the deleted `entry_type='artifact'`; left for its own check.
- `cascade_validator.go` rename (§3.2) — still pending; live code, fossil name.

## Verification

- `CGO_ENABLED=0 go build ./...`, `go vet ./...` — clean.
- `CGO_ENABLED=0 go test ./...` — all pass.
- Live smoke: killed commands unregistered (confirmed via `--help` Available
  Commands); `pudl model list/show/validate`, `pudl status`, `pudl repo init`,
  `pudl mu ingest-*`, `pudl guide models/mu` all work.

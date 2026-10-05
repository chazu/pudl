# ACUTE Loop & Workspace Implementation Complete

## Date
2026-03-25

## Summary

All 10 beads across 3 epics have been implemented, tested, and verified. The full ACUTE convergence loop is now closed, per-repo workspaces are functional, and BRICK metadata is load-bearing.

## Execution

Work was parallelized in 3 waves:

- **Wave 1** (parallel): B1.1, B1.2, B2.1, B3.1
- **Wave 2** (parallel, after Wave 1): B1.3, B2.2, B2.3, B2.4, B2.5
- **Wave 3** (after B1.3): B1.4

## New CLI Commands

- `pudl ingest-observe` — ingest mu observe results (NDJSON from stdin or file)
- `pudl ingest-manifest` — ingest mu build manifests (JSON from stdin or file)
- `pudl status [definition]` — show convergence status (table, detail, or JSON)

## New Packages

- `internal/workspace/` — workspace discovery (walk-up for .pudl/workspace.cue), context resolution

## Modified Packages

- `internal/mubridge/` — added ingest.go (observe), manifest.go (manifest ingestion)
- `internal/database/` — added status column migration, UpdateStatus, GetDefinitionStatuses, observe/manifest/artifact query methods with origin filtering
- `internal/drift/` — checker prefers observe results, updates status after check, supports origin filter
- `internal/definition/` — multi-path discovery with NewMultiDiscoverer, per-repo shadowing
- `internal/inference/` — multi-path schema loading with shadowing
- `internal/validator/` — multi-path schema validation
- `internal/importer/` — multi-path schema constructors
- `internal/schema/` — multi-path manager
- `internal/repo/` — generates workspace.cue, schema/, definitions/ on repo init
- `cmd/` — workspace context in root, scoped list/import/drift, new commands

## Test Coverage

All existing tests continue to pass. New tests added across:
- mubridge (ingest, manifest, export with BRICK)
- database (observe, manifest, status, artifacts)
- drift (observe preference, status updates)
- definition (multi-path, shadowing)
- inference (multi-path, shadowing)
- workspace (discovery, context)
- repo (workspace.cue generation)
- cmd (status output)

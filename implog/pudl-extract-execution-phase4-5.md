# Extract Execution Layer - Phases 4-5

## Summary

Deleted the execution packages and their dependencies now that all consumers were removed in Phases 1-3.

## Deleted Packages

- `internal/glojure/` (6 files, ~860 lines) — Glojure runtime, builtins, registry
- `internal/executor/` (5 files, ~990 lines) — Method execution, lifecycle dispatch, effects
- `internal/workflow/` (8 files, ~1,440 lines) — DAG runner, manifest, workflow parsing
- `internal/model/` (4 files, ~630 lines) — Model discovery, lifecycle, types
- `internal/artifact/` (2 files, ~250 lines) — Artifact store

## Other Deletions

- `test/acceptance/workflow_ssh_test.go` — acceptance test referencing deleted workflow/executor packages
- Empty `test/acceptance/` directory

## Dependency Cleanup

- Removed `github.com/glojurelang/glojure` from go.mod via `go mod tidy`
- All transitive dependencies of Glojure also removed from go.sum

## Impact

~4,170 lines of execution code removed across 25 files. pudl's internal package count dropped from ~28 to ~21.

Database artifact columns (`entry_type`, `definition`, `method`, `run_id`, `tags`) and `GetLatestArtifact()` remain in the database package — they serve `pudl data latest` and drift detection.

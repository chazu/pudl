# Extract Execution Layer - Phases 1-3

## Summary

Removed execution-related CLI commands and decoupled the drift checker from the execution layer, as the first step in extracting execution concerns from pudl into mu.

## Changes

### Phase 1: Deleted Execution CLI Commands (14 files)
- `cmd/method.go`, `cmd/method_list.go`, `cmd/method_run.go`
- `cmd/workflow.go`, `cmd/workflow_run.go`, `cmd/workflow_list.go`, `cmd/workflow_show.go`, `cmd/workflow_validate.go`, `cmd/workflow_history.go`
- `cmd/model.go`, `cmd/model_list.go`, `cmd/model_search.go`, `cmd/model_show.go`, `cmd/model_scaffold.go`
- Modified `cmd/repo.go` to remove model discovery from `runRepoValidateCommand()`

### Phase 2: Decoupled Drift Checker
- Rewrote `internal/drift/checker.go` — removed imports of `executor`, `model`, `workflow`
- `Checker` now takes only `definition.Discoverer` and `database.CatalogDB`
- Removed `Method`, `Refresh`, and `Tags` from `CheckOptions`
- Removed `Method` field from `DriftResult` struct
- Simplified `cmd/drift_check.go` — removed glojure/executor/model/vault setup
- Updated all drift tests

### Phase 3: Removed CUE Processor Glojure Dependency
- Deleted `cmd/process.go` (the `pudl process` command)
- Deleted `internal/cue/processor.go` and removed empty `internal/cue/` directory

## CLI Commands Removed
- `pudl method`, `pudl method list`, `pudl method run`
- `pudl workflow`, `pudl workflow run`, `pudl workflow list`, `pudl workflow show`, `pudl workflow validate`, `pudl workflow history`
- `pudl model`, `pudl model list`, `pudl model search`, `pudl model show`, `pudl model scaffold`
- `pudl process`

## CLI Commands Modified
- `pudl drift check` — simplified (no --method, --refresh, --tag flags)
- `pudl repo validate` — no longer reports model counts

## What Remains
The execution packages themselves (`internal/glojure/`, `internal/executor/`, `internal/workflow/`, `internal/model/`) still exist but are no longer imported by any cmd/ file. They will be deleted in the next PR (Phases 4-5).

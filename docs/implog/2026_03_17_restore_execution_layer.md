# Restore Execution Layer from git history

Date: 2026-03-17

## Summary

Restored the execution layer packages and CLI commands that were deleted in commit `f79297b`. The restoration integrates the pre-deletion code with the current codebase, including adapting to the `ModelRef` → `SchemaRef` rename.

## Phases completed

### Phase 1: go.mod dependencies
- Added `github.com/glojurelang/glojure v0.6.4` as direct dependency
- Added `golang.org/x/sync` as direct dependency (promoted from indirect)
- Ran `go mod tidy` to resolve transitive dependencies
- Note: `testcontainers-go` was removed by go mod tidy as no code references it directly

### Phase 2: op/ package (new)
- `op/functions.go` — `CustomFunction` interface
- `op/adapter.go` — `GlojureFunc` adapter and `GlojureCaller` interface

### Phase 3: internal/glojure/ (5 files + test)
- `runtime.go` — Glojure runtime lifecycle management
- `registry.go` — Unified function registry for Go and Glojure functions
- `builtins.go` — Registers pudl.core and pudl.http namespaces
- `ns_core.go` — Core functions: uppercase, lowercase, concat, format, now, env
- `ns_http.go` — HTTP functions: get, get-json, post, status
- `glojure_test.go` — Full test suite

### Phase 4: internal/model/ (4 files)
- `model.go`, `discovery.go`, `lifecycle.go`, `model_test.go`

### Phase 5: internal/artifact/ (2 files)
- `store.go`, `store_test.go`

### Phase 6: internal/cue/
- `processor.go` — CUE function processor

### Phase 7: internal/executor/ (6 files)
- `effects.go`, `effects_test.go`, `loader.go`, `executor_test.go` — verbatim
- `args.go`, `executor.go` — with `def.ModelRef` → `def.SchemaRef` adaptation

### Phase 8: internal/workflow/ (8 files)
- `workflow.go`, `dag.go`, `manifest.go`, `runner.go` + 4 test files — verbatim

### Phase 9: internal/drift/ updates
- Restored full pre-deletion `checker.go` (5-arg `NewChecker`, `CheckOptions.Method/Refresh/Tags`, `DriftResult.Method`)
- Applied `def.ModelRef` → `def.SchemaRef` adaptation
- Restored full pre-deletion `drift.go` (`DriftResult.Method` field added back)
- Restored `checker_test.go` and `drift_test.go`

### Phase 10: cmd/ files (16 files)
- Restored: `method.go`, `method_list.go`, `method_run.go`, `model.go`, `model_list.go`, `model_scaffold.go`, `model_search.go`, `model_show.go`, `process.go`, `workflow.go`, `workflow_history.go`, `workflow_list.go`, `workflow_run.go`, `workflow_show.go`, `workflow_validate.go`, `drift_check.go`
- Added model discoverer/count block back to `cmd/repo.go`

### Phase 11: Bootstrap CUE files (7 files)
- `internal/importer/bootstrap/definitions/` — http_def.cue, simple_def.cue, wired_defs.cue
- `internal/importer/bootstrap/pudl/model/` — model.cue + examples/

## Public API

### op package
- `CustomFunction` interface with `Execute(ctx, args) (interface{}, error)`
- `GlojureCaller` interface
- `GlojureFunc` struct

### internal/glojure
- `Runtime` — `New()`, `Init()`, `Eval()`, `RegisterGoFunc()`, `CallFunc()`
- `Registry` — `NewRegistry()`, `RegisterGo()`, `RegisterGlojure()`, `Get()`, `List()`
- `RegisterBuiltins(registry)` — registers pudl.core and pudl.http

### internal/drift (updated)
- `NewChecker(defDisc, modelDisc, db, exec, dataPath)` — 5-arg constructor
- `CheckOptions.Method`, `CheckOptions.Refresh`, `CheckOptions.Tags`
- `DriftResult.Method`

## Build status
- `go build ./...` — passes
- `go test ./internal/glojure/... ./internal/model/... ./internal/executor/... ./internal/workflow/...` — all pass

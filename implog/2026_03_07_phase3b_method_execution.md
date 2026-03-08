# Phase 3b: Method Execution Pipeline

**Date:** 2026-03-07

## Summary

Implemented the method execution pipeline — the ability to run Glojure `.clj` files as method implementations for model definitions, with lifecycle dispatch (qualifications gate actions, post-actions run after).

## New Packages

### `internal/executor/`

Core executor package with three files:

- **`executor.go`** (~190 lines) — `Executor` struct, `Run()` orchestration with lifecycle dispatch, qualification result parsing, `ListMethods()` for implementation status, Glojure persistent map conversion utilities.
- **`loader.go`** (~65 lines) — `.clj` file loading, unique function name generation to avoid collisions, `Eval`-based invocation.
- **`args.go`** (~25 lines) — Argument resolution from definition socket bindings + tags.

### CLI Commands

- **`cmd/method.go`** — Parent `pudl method` command
- **`cmd/method_run.go`** — `pudl method run <definition> <method> [--dry-run] [--skip-advice] [--tag k=v]`
- **`cmd/method_list.go`** — `pudl method list <definition>` — lists methods grouped by kind with implementation status

## Public API

### `executor.Executor`

```go
func New(rt *glojure.Runtime, reg *glojure.Registry, modelDisc *model.Discoverer, defDisc *definition.Discoverer, methodsDir string) *Executor
func (e *Executor) Run(ctx context.Context, opts RunOptions) (*RunResult, error)
func (e *Executor) ListMethods(defName string) ([]MethodStatus, error)
```

### Types

```go
type RunOptions struct {
    DefinitionName string
    MethodName     string
    DryRun         bool
    SkipAdvice     bool
    Tags           map[string]string
}

type RunResult struct {
    MethodName     string
    DefinitionName string
    Output         interface{}
    Qualifications []QualificationOutcome
    PostActions    []PostActionOutcome
}

type QualificationOutcome struct {
    Name    string
    Passed  bool
    Message string
}

type PostActionOutcome struct {
    Name   string
    Output interface{}
    Error  error
}

type MethodStatus struct {
    Name              string
    Kind              string
    Description       string
    HasImplementation bool
}
```

## Method File Convention

`.clj` files live at `<methodsDir>/<model-metadata-name>/<method-name>.clj` and must define `(defn run [args] ...)`.

## Execution Flow

1. Look up definition → model → lifecycle
2. Run qualifications (unless `--skip-advice`); abort if any return `{:passed false}`
3. If `--dry-run`, stop after qualifications
4. Execute action `.clj` file
5. Run post-actions (attribute/codegen methods)
6. Return `RunResult`

## Implementation Notes

- Functions are evaluated in the `user` namespace with unique names (atomic counter) to avoid collisions between method invocations.
- Glojure persistent maps (`*lang.Map`) are converted to `map[string]interface{}` using `lang.Seq` iteration.
- Keyword keys (`:passed`) are stripped of the colon prefix for Go map access.
- Arguments are passed via `RegisterGoFunc` as a closure and called from Eval.

## Tests (9 passing)

- `TestRunSimpleMethod` — end-to-end execution
- `TestQualificationPasses` — qualification passes, action runs
- `TestQualificationFails` — qualification fails, action aborted
- `TestDryRun` — qualifications only, no action
- `TestSkipAdvice` — qualifications skipped
- `TestMethodNotFound` — missing .clj file error
- `TestMethodList` — list methods with implementation status
- `TestLifecycleOrder` — qualifications → action → post-actions
- `TestTagsPassedToArgs` — tags merged into args

## Example Method Files

Created in `internal/importer/bootstrap/methods/`:
- `ec2_instance/list.clj` — mock EC2 instance list
- `ec2_instance/valid_credentials.clj` — mock credential check
- `simple/get_value.clj` — simple value extraction

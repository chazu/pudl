# Phase 6: Workflows — DAG Orchestration

## Summary

Implemented multi-step workflow orchestration with DAG-based dependency resolution,
concurrent step execution, and run manifest persistence.

## Public API

### Package: `internal/workflow`

#### Types
- `Workflow` — Parsed workflow definition with steps, description, abort policy
- `Step` — Single workflow step: definition, method, inputs, timeout, retries
- `DAG` — Step dependency graph with topological sort and ready-step detection
- `Manifest` / `StepManifest` — Run result records persisted as JSON
- `Runner` — Concurrent workflow executor with errgroup-based dispatch
- `StepExecutor` — Interface abstracting method execution (enables testing)
- `RunOptions` — Dry-run, tags, max concurrency settings
- `RunResult` / `StepResult` — Execution outcomes

#### Functions & Methods
- `NewDiscoverer(schemaPath)` — Creates workflow discoverer for `<schemaPath>/workflows/*.cue`
- `Discoverer.ListWorkflows()` — Discovers all workflows
- `Discoverer.GetWorkflow(name)` — Gets a specific workflow
- `BuildDAG(wf)` — Builds dependency graph from step input references
- `DAG.TopologicalSort()` — Kahn's algorithm, returns error on cycles
- `DAG.GetReadySteps(completed)` — Returns steps with all deps satisfied
- `DAG.GetDependencies(name)` — Lists what a step depends on
- `NewManifestStore(dataPath)` — Creates manifest store at `.runs/<workflow>/`
- `ManifestStore.Save/List/Get` — CRUD for run manifests
- `NewRunner(exec, db, dataPath)` — Creates concurrent workflow runner
- `Runner.Run(ctx, wf, opts)` — Executes workflow with output threading

### CLI Commands
- `pudl workflow list` — List discovered workflows
- `pudl workflow show <name>` — Display steps, DAG, topological order
- `pudl workflow validate <name>` — Validate structure and references
- `pudl workflow run <name>` — Execute with concurrent dispatch
- `pudl workflow history <name>` — Show past run manifests

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `internal/workflow/workflow.go` | ~220 | Types + CUE parser |
| `internal/workflow/dag.go` | ~155 | DAG builder + topo sort |
| `internal/workflow/manifest.go` | ~115 | Run manifest persistence |
| `internal/workflow/runner.go` | ~265 | Concurrent execution engine |
| `internal/workflow/workflow_test.go` | ~170 | Parser tests |
| `internal/workflow/dag_test.go` | ~215 | DAG + GetReadySteps tests |
| `internal/workflow/runner_test.go` | ~290 | Runner tests (mock executor) |
| `internal/workflow/manifest_test.go` | ~95 | Manifest round-trip tests |
| `cmd/workflow.go` | ~25 | Parent command |
| `cmd/workflow_run.go` | ~120 | Run CLI |
| `cmd/workflow_list.go` | ~50 | List CLI |
| `cmd/workflow_show.go` | ~75 | Show CLI |
| `cmd/workflow_validate.go` | ~55 | Validate CLI |
| `cmd/workflow_history.go` | ~60 | History CLI |
| `test/acceptance/workflow_ssh_test.go` | ~195 | SSH acceptance test |

## Key Design Decisions

- **StepExecutor interface** — Decouples runner from concrete executor for unit testing
- **sync.Map for output threading** — Write-once/read-many pattern for concurrent steps
- **errgroup with SetLimit** — Natural concurrency control + clean abort
- **Text-based CUE parsing** — Consistent with model/definition discovery patterns
- **Flat JSON manifests** — One file per run, human-readable for debugging

## Test Results

- 25 unit tests: all pass
- Covers: parser, DAG (linear/diamond/independent/cycle/unknown refs/ready steps),
  runner (success/concurrency/abort/retry/input resolution/max concurrency/dry-run),
  manifest (save/load/list round-trip)

# Phase 8: Agent Integration & Skill Files

**Date:** 2026-03-07

## Summary

Phase 8 is the capstone: enabling AI agents to discover, write, and orchestrate PUDL resources. This includes the effect description pattern, embedded skill files, model search/scaffold CLI commands, and extension model discovery.

## What Was Built

### Effect System (`internal/executor/effects.go`)

Types and parsing for method effects — a pattern where methods return `pudl/effects` descriptions rather than executing side effects directly:

- `Effect` — `Kind`, `Description`, `Params` for describing intended operations
- `EffectOutcome` — records execution status per effect
- `ParseEffects(output)` — extracts effects from method output maps
- `FormatEffect()`, `FormatEffectOutcome()` — human-readable formatting

**Executor integration:** `RunResult` now carries `Effects` and `EffectOutcomes`. After method execution, the executor checks output for `pudl/effects` and stores them on the result. In dry-run mode, effects are listed but marked "skipped".

### Skill File Embed System (`internal/skills/`)

Five markdown skill files embedded into the binary via `//go:embed`:

| Skill | Content |
|-------|---------|
| `pudl-core` | CLI overview, repository layout, common commands |
| `pudl-definitions` | Writing CUE definitions, socket wiring, vault refs |
| `pudl-methods` | Writing Glojure methods, lifecycle, builtins, effect pattern |
| `pudl-workflows` | Composing workflow DAGs, step fields, dependency resolution |
| `pudl-models` | Defining models, sockets, auth, extension models, scaffold |

**API:**
- `skills.ListSkills() ([]SkillInfo, error)` — list embedded skills
- `skills.ReadSkill(name) ([]byte, error)` — read skill content
- `skills.WriteSkills(targetDir) error` — write all skills as `<dir>/<name>/SKILL.md`

### Init Integration

`pudl init` now writes skill files to `.claude/skills/` if a `.claude/` directory exists in the project root or current working directory.

### Model Search (`cmd/model_search.go`)

`pudl model search <query>` — case-insensitive substring match across:
- Model name, metadata name, description, category
- Method names, socket names

### Model Scaffold (`cmd/model_scaffold.go`)

`pudl model scaffold <name> [flags]` generates:
1. `models/<name>/<name>.cue` — Model CUE file with resource schema, metadata, methods
2. `methods/<name>/<method>.clj` — Glojure method stubs
3. `definitions/<name>_def.cue` — Definition template

Flags: `--category`, `--methods`, `--sockets`, `--auth`

### Extension Model Discovery

`model.Discoverer.ListModels()` now also walks `extensions/models/` under the schema path. Graceful no-op if the directory doesn't exist.

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `internal/executor/effects.go` | 130 | Effect types + parsing |
| `internal/executor/effects_test.go` | 140 | Effect tests |
| `internal/skills/skills.go` | 70 | Embed + write skill files |
| `internal/skills/skills_test.go` | 60 | Skill embed tests |
| `internal/skills/files/pudl-core.md` | 70 | CLI + repo layout |
| `internal/skills/files/pudl-definitions.md` | 80 | Definition writing |
| `internal/skills/files/pudl-methods.md` | 110 | Method writing |
| `internal/skills/files/pudl-workflows.md` | 80 | Workflow composition |
| `internal/skills/files/pudl-models.md` | 120 | Model definition |
| `cmd/model_search.go` | 90 | Search command |
| `cmd/model_scaffold.go` | 170 | Scaffold command |

## Files Modified

| File | Change |
|------|--------|
| `internal/executor/executor.go` | Added `Effects`/`EffectOutcomes` to `RunResult`, effect detection after `loadAndRun()` |
| `internal/model/discovery.go` | Added `extensions/models/` walk in `ListModels()` |
| `internal/init/init.go` | Added `writeSkillFiles()` call + import |
| `cmd/method_run.go` | Display effects and outcomes after result output |
| `docs/plan.md` | Marked Phase 8 complete |

## Public API

### Effect Types
```go
executor.Effect{Kind, Description, Params}
executor.EffectOutcome{Effect, Status, Output, Error}
executor.ParseEffects(output interface{}) ([]Effect, bool)
executor.FormatEffect(e Effect) string
executor.FormatEffectOutcome(eo EffectOutcome) string
```

### Skill System
```go
skills.ListSkills() ([]SkillInfo, error)
skills.ReadSkill(name string) ([]byte, error)
skills.WriteSkills(targetDir string) error
```

### CLI Commands
```
pudl model search <query>
pudl model scaffold <name> [--category C] [--methods m1,m2] [--sockets s1:input,s2:output] [--auth method]
```

## Verification

- `go build ./...` — compiles successfully
- `go test ./...` — all tests pass (executor, skills, model, plus full suite)

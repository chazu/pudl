# Phase 2: Definitions — Named Resource Instances

**Date:** 2026-03-06

## Summary

Implemented definitions — named instances of models with concrete configuration and socket wiring. Definitions live in `~/.pudl/schema/definitions/` as CUE files that import and unify against model schemas.

## Public API

### `internal/definition` Package

**Types:**
- `DefinitionInfo` — Name, ModelRef, Package, FilePath, SocketBindings, Validated
- `ValidationResult` — Name, Valid, Errors
- `Graph` — Dependency graph with topological sort

**Discovery:**
- `NewDiscoverer(schemaPath string) *Discoverer`
- `ListDefinitions() ([]DefinitionInfo, error)`
- `GetDefinition(name string) (*DefinitionInfo, error)`

**Validation:**
- `NewValidator(schemaPath string) *Validator`
- `ValidateDefinition(name string) (*ValidationResult, error)`
- `ValidateAll() ([]ValidationResult, error)`

**Graph:**
- `BuildGraph(definitions []DefinitionInfo) *Graph`
- `TopologicalSort() ([]string, error)` — Kahn's algorithm, errors on cycles
- `GetDependencies(name string) []string`
- `GetDependents(name string) []string`

### CLI Commands

- `pudl definition list [--verbose] [--model <ref>]`
- `pudl definition show <name>`
- `pudl definition validate [name]`
- `pudl definition graph`
- `pudl repo validate`

## Files Created

| File | Purpose |
|------|---------|
| `internal/definition/definition.go` | Core types |
| `internal/definition/discovery.go` | Text-based definition discovery |
| `internal/definition/validator.go` | CUE-based validation |
| `internal/definition/graph.go` | Dependency graph + topological sort |
| `internal/definition/definition_test.go` | Tests (14 test cases) |
| `internal/importer/bootstrap/definitions/simple_def.cue` | Simple definition example |
| `internal/importer/bootstrap/definitions/http_def.cue` | HTTP endpoint definition |
| `internal/importer/bootstrap/definitions/wired_defs.cue` | Socket wiring example |
| `cmd/definition.go` | Parent definition command |
| `cmd/definition_list.go` | List command |
| `cmd/definition_show.go` | Show command |
| `cmd/definition_validate.go` | Validate command |
| `cmd/definition_graph.go` | Graph command |
| `cmd/repo.go` | Repository validate command |
| `docs/definition-authoring.md` | Definition authoring guide |

## Files Modified

| File | Change |
|------|--------|
| `internal/init/init.go` | Added `definitions/` directory creation |
| `internal/schema/manager.go` | Skip `definitions/` in `ListSchemas()` |
| `docs/concepts.md` | Added definitions section |
| `docs/cli-reference.md` | Added definition + repo commands |
| `docs/README.md` | Added definition-authoring.md link |
| `docs/plan.md` | Marked Phase 2 complete |

## Design Decisions

- **Text-based discovery** — Matches model discovery pattern. Regex-based detection of `#*Model &` unification and `_model:` markers.
- **Socket binding detection** — Cross-definition references (`name.outputs.field`) parsed from definition bodies.
- **Kahn's algorithm** — Standard topological sort with cycle detection for dependency ordering.
- **Validation via CUE loader** — Reuses `CUEModuleLoader.LoadAllModules()` which loads all CUE files including definitions. If the load succeeds, definitions are valid.
- **Bootstrap definitions** — Embedded via `go:embed` alongside model examples, copied during `pudl init`.

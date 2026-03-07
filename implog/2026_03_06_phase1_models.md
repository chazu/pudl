# Phase 1: Models — Composing Schemas with Behavior

**Date:** 2026-03-06

## Summary

Implemented models as a new concept separate from schemas. Models reference CUE schemas and add methods, sockets, authentication, and metadata. Schemas remain pure data shapes — models layer operational behavior on top.

## Design Decisions

- **Text-based CUE parsing** for model discovery rather than full CUE loader evaluation. This avoids the complexity of the CUE runtime while still extracting all needed information from model files. Uses regex-based extraction of definitions that reference `#Model`.
- **Bootstrap CUE types** in `pudl/model/model.cue` define the canonical model schema (`#Model`, `#Method`, `#Socket`, `#AuthConfig`, `#ModelMetadata`, `#QualificationResult`). Example models unify against these types for validation.
- **CUE import paths** use `pudl.schemas/pudl/model` (the full module-qualified path) to work within the existing CUE module system (`pudl.schemas@v0`).
- **Models coexist with schemas** in the schema repository. The model discoverer walks the full schema path to find model definitions.
- **Lifecycle resolution** is a pure function: given a model and method name, it finds all qualification methods that block it and returns the execution order.

## Public API

### `internal/model` package

```go
// Types
type ModelInfo struct { Name, Package, FilePath, Schema, State string; Metadata; Methods; Sockets; Auth }
type ModelMetadata struct { Name, Description, Category, Icon string }
type Method struct { Kind, Description, Timeout string; Retries int; Blocks []string }
type Socket struct { Direction, Description string; Required bool }
type AuthConfig struct { Method string }
type Lifecycle struct { Qualifications, PostActions []string; Action string }

// Discovery
func NewDiscoverer(schemaPath string) *Discoverer
func (d *Discoverer) ListModels() ([]ModelInfo, error)
func (d *Discoverer) GetModel(name string) (*ModelInfo, error)

// Lifecycle
func ResolveLifecycle(model *ModelInfo, methodName string) (*Lifecycle, error)
```

### CLI Commands

- `pudl model list [--category <cat>] [--verbose]` — List all models
- `pudl model show <model-name>` — Display model details with tab completion

### CUE Types

- `#Model` — Top-level model type
- `#Method` — Method with kind (action/qualification/attribute/codegen)
- `#Socket` — Typed input/output port
- `#AuthConfig` — Authentication configuration
- `#ModelMetadata` — Model descriptive metadata
- `#QualificationResult` — Standard qualification return type

## Files Created

| File | Purpose |
|------|---------|
| `internal/importer/bootstrap/pudl/model/model.cue` | Bootstrap CUE base types |
| `internal/importer/bootstrap/pudl/model/examples/aws_ec2.cue` | EC2 example model |
| `internal/importer/bootstrap/pudl/model/examples/http_endpoint.cue` | HTTP endpoint example |
| `internal/importer/bootstrap/pudl/model/examples/simple.cue` | Minimal example model |
| `internal/model/model.go` | Go types |
| `internal/model/discovery.go` | Model discovery and parsing |
| `internal/model/lifecycle.go` | Qualification lifecycle resolution |
| `internal/model/model_test.go` | Tests (7 test functions, all passing) |
| `cmd/model.go` | Parent model command |
| `cmd/model_list.go` | `pudl model list` |
| `cmd/model_show.go` | `pudl model show` with tab completion |
| `docs/model-authoring.md` | Model authoring guide |

## Files Modified

| File | Change |
|------|--------|
| `internal/importer/cue_schemas.go` | Added model bootstrap check in `ensureBasicSchemas()` |
| `docs/concepts.md` | Added Models section with schema/model/definition hierarchy |
| `docs/cli-reference.md` | Added `pudl model list` and `pudl model show` |
| `docs/README.md` | Added model-authoring.md to documentation index |
| `docs/plan.md` | Marked Phase 1 as complete |

## Test Results

All existing tests continue to pass. New model package tests:
- TestListModels — discovers all 3 example models
- TestGetModel — finds by name, errors on missing
- TestEC2ModelParsing — full model with methods, sockets, auth
- TestHTTPModelParsing — model with qualifications and bearer auth
- TestSimpleModelParsing — minimal model (no sockets, no auth)
- TestLifecycleResolution — qualification blocking, multi-qualification
- TestLifecycleMinimalModel — model with no qualifications

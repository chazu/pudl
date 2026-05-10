# B2.5 — Workspace-Aware Schema Resolution

## Summary

Extended schema inference, validation, and import to search multiple schema
directories in priority order. Per-repo `.pudl/schema/` now shadows global
`~/.pudl/schema/` when schemas share the same fully-qualified name.

## Changes

### internal/inference/inference.go
- `SchemaInferrer` now holds `schemaPaths []string` and `loaders []*CUEModuleLoader`
- `NewSchemaInferrer(schemaPaths ...string)` — variadic constructor; loads schemas
  from each path in order with first-found-wins shadowing
- `loadSchemasFromPaths()` — shared helper for initial load and reload
- `Reload()` — reloads from all configured paths

### internal/validator/cascade_validator.go
- `CascadeValidator` now holds `schemaPaths []string` and `loaders []*CUEModuleLoader`
- `NewCascadeValidator(schemaPaths ...string)` — variadic constructor with
  multi-path loading and per-path integrity validation
- `GetModuleInfo()` — searches all loaders
- `ReloadModules()` — reloads from all paths

### internal/validator/validation_service.go
- `NewValidationService(schemaPaths ...string)` — variadic, passes through

### internal/importer/importer.go
- `Importer` struct gains `schemaPaths []string`
- `New()` preserved for backward compatibility (delegates to `NewWithSchemaPaths`)
- `NewWithSchemaPaths(dataPath, pudlHome string, schemaPaths ...string)` — multi-path constructor

### internal/importer/enhanced_importer.go
- `NewEnhancedImporter()` preserved for backward compatibility
- `NewEnhancedImporterWithSchemaPaths(dataPath, configDir string, schemaPaths ...string)` added

### internal/schema/manager.go
- `Manager` struct gains `schemaPaths []string`
- `NewManagerWithPaths(schemaPaths ...string)` — multi-path constructor
- `ListAllSchemas()` — lists schemas from all paths with shadowing

### internal/database/catalog_migrations.go
- Added `ensureStatusColumn()` — fixes pre-existing missing method

## Public API

```go
// Inference — variadic schema paths
inference.NewSchemaInferrer(schemaPaths ...string) (*SchemaInferrer, error)

// Validation — variadic schema paths
validator.NewCascadeValidator(schemaPaths ...string) (*CascadeValidator, error)
validator.NewValidationService(schemaPaths ...string) (*ValidationService, error)

// Import — multi-path constructors
importer.NewWithSchemaPaths(dataPath, pudlHome string, schemaPaths ...string) (*Importer, error)
importer.NewEnhancedImporterWithSchemaPaths(dataPath, configDir string, schemaPaths ...string) (*EnhancedImporter, error)

// Schema management — multi-path constructor
schema.NewManagerWithPaths(schemaPaths ...string) *Manager
schema.Manager.ListAllSchemas() (map[string][]SchemaInfo, error)
```

## Tests Added

- `TestInferrer_MultiPath` — schemas from two directories are all found
- `TestInferrer_Shadowing` — same schema name in per-repo and global; per-repo wins, no duplicates
- `TestInferrer_FallbackToGlobal` — per-repo has no match, global schemas still used
- `TestInferrer_SinglePathBackwardCompat` — single path still works
- `TestInferrer_SkipsInaccessiblePaths` — nonexistent paths skipped gracefully

## Backward Compatibility

All existing single-path constructors are preserved and delegate to the new
multi-path implementations. No call sites were changed; they continue to pass
a single schema path and work identically.

# Schema Inference Refactor

**Date:** 2025-11-25
**Summary:** Removed hard-coded schema detection rules and replaced with CUE-based inference system.

## Overview

This refactor removed all hard-coded schema detection logic (AWS, K8s patterns) from Go code and replaced it with a pure CUE unification-based inference system. Schemas are now derived entirely from the user's CUE schema repository.

## Changes Made

### New Package: `internal/inference/`

Created a new inference package with three modules:

1. **`graph.go`** - Inheritance graph for schema specificity
   - `InheritanceGraph` struct tracking parent-child relationships
   - `BuildInheritanceGraph(metadata)` - builds graph from `_pudl.base_schema` metadata
   - `GetMostSpecificFirst()` - returns schemas ordered by specificity (leaves first)
   - `GetCascadeChain(schema)` - returns inheritance path to root

2. **`heuristics.go`** - Candidate selection
   - `InferenceHints` struct with Origin and Format hints
   - `CandidateScore` struct with Schema, Score, and Reason
   - `SelectCandidates(data, hints, metadata, graph)` - returns ordered candidates
   - Uses `identity_fields` from `_pudl` metadata for matching
   - Requires 2+ keyword matches for origin hints

3. **`inference.go`** - Core inference logic
   - `SchemaInferrer` struct wrapping CUE loader
   - `NewSchemaInferrer(schemaPath)` - creates inferrer, loads all schemas
   - `Infer(data, hints)` - returns `InferenceResult` with best matching schema
   - `Reload()` - reloads schemas (for hot reload)
   - `GetAvailableSchemas()` - lists loaded schema names

### Public API

```go
// Create inferrer
inferrer, err := inference.NewSchemaInferrer("/path/to/schemas")

// Infer schema for data
result, err := inferrer.Infer(data, inference.InferenceHints{
    Origin: "aws-ec2",
    Format: "json",
})

// Result contains:
result.Schema      // e.g., "aws.#EC2Instance"
result.Confidence  // 0.0-1.0
result.CascadePath // inheritance chain
result.MatchedAt   // position in candidate list
result.Reason      // explanation
```

### Bootstrap Schemas (CUE files)

Moved bootstrap schemas from embedded Go strings to actual CUE files:
- `internal/importer/bootstrap/pudl/unknown/catchall.cue` - CatchAll schema
- `internal/importer/bootstrap/pudl/collections/collections.cue` - Collection schemas

Uses Go embed (`//go:embed bootstrap`) to include at build time.

### Removed Components

- **`internal/rules/` package** - Deleted entirely
- **`assignSchema()` function** - Removed from `internal/importer/schema.go`
- **`hasFields()` helper** - Removed
- **`basicSchemaDefinitions` map** - Removed
- **`loadDefaultPatterns()` hard-coded patterns** - Removed from streaming/schema_detector.go

### Updated Components

- **`internal/importer/importer.go`** - Uses `inference.SchemaInferrer` instead of rules
- **`internal/importer/enhanced_importer.go`** - Uses inference for schema assignment
- **`internal/importer/cue_schemas.go`** - Uses embedded CUE files via `fs.WalkDir`
- **`cmd/import.go`** - Removed Zygomys flag and rule engine override
- **`test/testutil/mock_services.go`** - `MockSchemaInferrer` replaces `MockRuleEngine`

### Test Infrastructure Updates

- Added CUE module setup to test fixtures (`testutil/temp_dirs.go`)
- Added CUE module setup to integration test suite
- Added CUE module setup to system tests
- Updated tests to expect catchall when no specific schemas present

## Design Decisions

1. **CUE Unification as Source of Truth**: Schema matching uses `schema.Unify(data).Validate()` - if it validates, it matches.

2. **Specificity via Inheritance**: More specific schemas are tried first, based on CUE's `#Specific: #Base & {...}` pattern.

3. **Heuristic Pre-filtering**: Candidates are scored by identity field matches and origin hints to avoid trying every schema.

4. **Metadata in `_pudl`**: Each schema can include `_pudl` metadata for identity_fields, cascade_priority, base_schema, etc.

5. **Bootstrap Schemas as CUE**: The app populates bootstrap schemas on setup, but they live as actual CUE files.

## Testing

All existing tests pass with updates:
- Removed tests for deleted hard-coded rules
- Updated tests to work with inference-based approach
- Added tests for new inference package

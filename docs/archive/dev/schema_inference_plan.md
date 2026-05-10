# Schema Inference System - Implementation Plan

## Overview

Replace hard-coded schema detection rules with a CUE-driven inference system that determines the best matching schema by attempting unification against schemas from the user's schema repository.

## Current State (Problems)

### Hard-coded Schema Detection in Three Locations:

1. **`internal/importer/schema.go:13-131`** - `assignSchema()` function with hard-coded AWS/K8s field detection
2. **`internal/rules/legacy.go`** - `LegacyRuleEngine` duplicating the same hard-coded logic
3. **`internal/streaming/schema_detector.go:292-350`** - `loadDefaultPatterns()` with hard-coded patterns

### Additional Hard-coded Elements:

4. **`internal/importer/schema.go:171-200`** - `basicSchemaDefinitions` map with hard-coded schema metadata
5. **`internal/importer/cue_schemas.go`** - Bootstrap schemas as Go string literals

## Target State

- Schema inference driven entirely by CUE schemas in the user's schema repository
- Specificity determined by CUE inheritance relationships (`#Specific: #Base & {...}`)
- Heuristics for candidate selection using `_pudl` metadata (identity_fields, cascade_priority)
- Bootstrap schemas written as actual CUE files on workspace init

## Existing Infrastructure to Leverage

The `internal/validator/` package already has:

- **`CUEModuleLoader`** - Loads all CUE schemas from schema directory
- **`CascadeValidator`** - Does CUE unification (`schema.Unify(data)`)
- **`SchemaMetadata`** - Extracts `_pudl` metadata including:
  - `cascade_priority` (int) - higher = more specific
  - `base_schema` (string) - parent schema reference
  - `identity_fields` ([]string) - key fields for this schema type
  - `cascade_fallback` ([]string) - explicit fallback chain

## Implementation Steps

### Phase 1: Create Inference Package

**New file: `internal/inference/inference.go`**

```go
package inference

type SchemaInferrer struct {
    loader    *validator.CUEModuleLoader
    modules   map[string]*validator.LoadedModule
    schemas   map[string]cue.Value
    metadata  map[string]validator.SchemaMetadata
    graph     *InheritanceGraph
}

type InferenceResult struct {
    Schema       string   // Best matching schema name
    Confidence   float64  // 0.0-1.0
    CascadePath  []string // Schemas tried in order
    MatchedAt    int      // Index in cascade path where match occurred
}

func NewSchemaInferrer(schemaPath string) (*SchemaInferrer, error)
func (si *SchemaInferrer) Infer(data interface{}, hints InferenceHints) (*InferenceResult, error)
func (si *SchemaInferrer) Reload() error
```

**New file: `internal/inference/graph.go`**

```go
package inference

// InheritanceGraph tracks schema inheritance relationships
type InheritanceGraph struct {
    children map[string][]string // parent -> children (more specific)
    parents  map[string]string   // child -> parent (less specific)
    roots    []string            // Schemas with no parent
    leaves   []string            // Schemas with no children (most specific)
}

func BuildInheritanceGraph(metadata map[string]validator.SchemaMetadata) *InheritanceGraph
func (g *InheritanceGraph) GetMostSpecificFirst() []string
func (g *InheritanceGraph) GetCascadeChain(schema string) []string
```

**New file: `internal/inference/heuristics.go`**

```go
package inference

type InferenceHints struct {
    Origin string            // e.g., "aws-ec2-instances"
    Format string            // e.g., "json"
    Fields []string          // Top-level field names present in data
}

// CandidateSelector narrows down schemas to try based on heuristics
func (si *SchemaInferrer) SelectCandidates(data interface{}, hints InferenceHints) []string
```

### Phase 2: Implement Core Inference Logic

**`internal/inference/inference.go` - Infer() method:**

1. Extract hints from data (top-level field names)
2. Select candidate schemas using heuristics:
   - Match `identity_fields` from `_pudl` metadata against data fields
   - Boost schemas whose identity_fields are all present
   - Consider origin hints if available
3. Sort candidates by specificity (leaves first, using inheritance graph)
4. For each candidate:
   - Convert data to CUE value
   - Attempt `schema.Unify(data).Validate()`
   - If success: return this schema
   - If failure: continue to next candidate
5. Fall back to `unknown.#CatchAll`

### Phase 3: Update Importer

**Modify `internal/importer/importer.go`:**

1. Add `inferrer *inference.SchemaInferrer` field to `Importer` struct
2. Initialize inferrer in `NewImporter()` using schema path
3. Replace call to `assignSchema()` with `inferrer.Infer()`

**Delete from `internal/importer/schema.go`:**
- `assignSchema()` function (lines 13-131)
- `hasFields()` helper (lines 134-141)
- `basicSchemaDefinitions` map (lines 171-200)
- `SchemaDefinition` type (lines 203-209)
- `getSchemaDefinition()` function (lines 212-215)

Keep only `updateCatalog()` function.

### Phase 4: Update Streaming Parser

**Modify `internal/streaming/schema_detector.go`:**

1. Replace `SimpleSchemaDetector` with adapter to `inference.SchemaInferrer`
2. Or: Have streaming parser use inference package directly
3. Delete `loadDefaultPatterns()` function (lines 292-350)

**Option A: Adapter approach**
```go
type StreamingSchemaDetector struct {
    inferrer *inference.SchemaInferrer
}

func (d *StreamingSchemaDetector) DetectSchema(chunk *ProcessedChunk) (*SchemaDetection, error) {
    // Use first object from chunk for inference
    if len(chunk.Objects) > 0 {
        result, err := d.inferrer.Infer(chunk.Objects[0], inference.InferenceHints{
            Format: chunk.Format,
        })
        // Convert to SchemaDetection
    }
}
```

### Phase 5: Bootstrap Schemas as CUE Files

**Modify `internal/importer/cue_schemas.go`:**

Convert `createBasicSchemas()` to write proper CUE files with full `_pudl` metadata:

1. `pudl/unknown/catchall.cue` - Catch-all schema (lowest priority)
2. `pudl/collections/collections.cue` - Collection schemas

Ensure these are written to the schema repo on `pudl init`, not embedded as Go strings at runtime.

**Move bootstrap schema content to `internal/schema/bootstrap/` as actual `.cue` files** that get copied to the user's schema repo on init. This makes them editable and version-controlled.

### Phase 6: Remove Rules Package

**Delete entire `internal/rules/` directory:**
- `config.go`
- `errors.go`
- `interfaces.go`
- `legacy.go`
- `manager.go`
- `manager_test.go`
- `zygomys.go`

**Update any imports** that reference the rules package (likely in importer).

### Phase 7: Testing

1. **Unit tests for inference package:**
   - `inference_test.go` - Core inference logic
   - `graph_test.go` - Inheritance graph building
   - `heuristics_test.go` - Candidate selection

2. **Integration tests:**
   - Test inference against real CUE schemas
   - Test cascade behavior (specific → generic → catchall)
   - Test with AWS EC2, K8s Pod, and unknown data

3. **Update existing tests:**
   - Remove tests that depend on rules package
   - Update importer tests to work with inference

## File Changes Summary

### New Files
- `internal/inference/inference.go`
- `internal/inference/graph.go`
- `internal/inference/heuristics.go`
- `internal/inference/inference_test.go`
- `internal/schema/bootstrap/unknown/catchall.cue`
- `internal/schema/bootstrap/collections/collections.cue`

### Modified Files
- `internal/importer/importer.go` - Use inference instead of assignSchema
- `internal/importer/schema.go` - Remove hard-coded logic, keep updateCatalog
- `internal/importer/cue_schemas.go` - Copy bootstrap schemas instead of generating
- `internal/streaming/schema_detector.go` - Use inference package
- `internal/streaming/cue_integration.go` - Update if needed

### Deleted Files
- `internal/rules/config.go`
- `internal/rules/errors.go`
- `internal/rules/interfaces.go`
- `internal/rules/legacy.go`
- `internal/rules/manager.go`
- `internal/rules/manager_test.go`
- `internal/rules/zygomys.go`

## Schema Metadata Requirements

For inference to work well, schemas should include `_pudl` metadata:

```cue
#EC2Instance: {
    _pudl: {
        schema_type: "base"
        resource_type: "aws.ec2.instance"
        cascade_priority: 80
        identity_fields: ["InstanceId"]  // Used for heuristic matching
        tracked_fields: ["State", "InstanceType", "Tags"]
    }

    InstanceId: string
    State: {...}
    InstanceType: string
    // ...
}

#CompliantEC2Instance: #EC2Instance & {
    _pudl: {
        schema_type: "policy"
        resource_type: "aws.ec2.instance"
        base_schema: "aws.#EC2Instance"
        cascade_priority: 90  // Higher = more specific
        cascade_fallback: ["aws.#EC2Instance", "unknown.#CatchAll"]
    }

    // Additional compliance constraints
    Tags: [...{Key: string, Value: string}] & list.MinItems(1)
}
```

## Inference Algorithm (Detailed)

```
Infer(data, hints):
    1. fields = extractTopLevelFields(data)

    2. candidates = []
       for schema in allSchemas:
           meta = getMetadata(schema)
           if meta.identity_fields all present in fields:
               candidates.append(schema)

       // If no candidates matched by identity_fields, try all schemas
       if len(candidates) == 0:
           candidates = allSchemas

    3. // Sort by specificity (most specific first)
       sort candidates by:
           - inheritance depth (leaves before roots)
           - cascade_priority (higher first)

    4. for schema in candidates:
           cueData = convertToCUE(data)
           unified = schema.Unify(cueData)
           if unified.Validate() == nil:
               return InferenceResult{
                   Schema: schema,
                   Confidence: calculateConfidence(schema, fields),
                   CascadePath: candidates[:index+1],
               }

    5. return InferenceResult{
           Schema: "unknown.#CatchAll",
           Confidence: 0.1,
       }
```

## Open Questions

1. **Performance**: Loading all schemas on startup - acceptable for typical schema repo sizes?
2. **Caching**: Should we cache inference results by data fingerprint?
3. **Partial matches**: Should we report "matched N of M identity fields" for debugging?

## Success Criteria

- [x] No hard-coded schema names in Go code (except `unknown.#CatchAll` as fallback)
- [x] Adding new schema types only requires adding CUE files, no Go changes
- [x] Inheritance-based specificity works correctly
- [x] Existing tests pass (with necessary updates)
- [x] Performance acceptable (inference < 100ms for typical data)

## Implementation Status: COMPLETED (2025-11-25)

All phases implemented:
- Phase 1: Created `internal/inference/` package (graph.go, heuristics.go, inference.go)
- Phase 2: Implemented core inference with CUE unification
- Phase 3: Updated importer to use inference
- Phase 4: Updated streaming parser (removed hard-coded patterns)
- Phase 5: Bootstrap schemas as CUE files (`internal/importer/bootstrap/`)
- Phase 6: Deleted `internal/rules/` package
- Phase 7: All tests passing

See `implog/schema-inference-refactor.md` for detailed implementation notes.

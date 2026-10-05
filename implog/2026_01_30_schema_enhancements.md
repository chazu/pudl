# Schema Enhancements - 2026-01-30

## Summary

Implemented two enhancements to the schema system:
1. Added inline documentation comments to generated schema metadata fields
2. Added `pudl schema reinfer` command for batch re-inference of existing data

## Changes

### 1. Schema Metadata Comments (pudl-o09)

Added inline comments to the `_pudl` metadata block explaining valid values:

**Files Modified:**
- `internal/schemagen/generator.go` - Added comments in `generateCUEContent()` function
- `internal/review/schema.cue.tmpl` - Added comments to template

**Metadata fields documented:**
- `schema_type`: Valid values "base", "collection", "policy", "catchall"
- `resource_type`: Format `<package>.<type>` - identifies the resource type
- `cascade_priority`: 0-1000, higher = more specific (catchall=0, base=100, policy=200+)
- `cascade_fallback`: Schemas to try if this doesn't match
- `compliance_level`: Valid values "strict", "warn", "permissive"

### 2. Schema Reinfer Command (pudl-9kj)

Added `pudl schema reinfer` command to re-run schema inference on existing catalog entries.

**Files Modified:**
- `cmd/schema.go` - Added command, flags, and implementation

**Public API:**

```
pudl schema reinfer [flags]

Flags:
  --all             Re-infer schemas for all catalog entries
  --entry string    Re-infer schema for specific entry by proquint ID
  --schema string   Re-infer only entries currently assigned to this schema
  --origin string   Re-infer only entries from a specific origin
  --dry-run         Show what would change without applying updates
  --force           Apply changes without confirmation prompt
```

**Use Cases:**
- Re-assign entries after adding new schemas
- Update assignments after modifying cascade priorities
- Batch update schema assignments without interactive review

## Testing

- Build passes: `go build ./...`
- Command help verified: `./pudl schema reinfer --help`
- Note: Pre-existing test failure in `internal/ui/ui_test.go` unrelated to these changes

---

## Bug Fix: Collection Type Awareness in Schema Inference

### Problem
When running `pudl schema reinfer` on a collection entry, the inference would incorrectly suggest `#CatchAll` (an item-only schema) instead of a collection-appropriate schema.

### Root Cause
1. `InferenceHints` didn't include collection type information
2. The heuristics didn't filter schemas based on collection type
3. The fallback logic always returned `#CatchAll` regardless of entry type

### Solution
1. Added `CollectionType` field to `InferenceHints` struct
2. Updated `scoreCandidate()` to filter schemas based on collection type:
   - Collections only match `schema_type: "collection"` schemas
   - Items don't match collection or collection_item schemas
3. Added `findFallbackSchema()` to return collection-appropriate fallbacks
4. Updated reinfer command to pass collection type from catalog entry

### Follow-up: Structural Type Detection for Collections

The initial fix relied on `schema_type: "collection"` metadata, but this approach is flawed because:
- Collection schemas like `#CatchAllCollection: [...]` are structurally arrays
- Arrays can't have `_pudl` metadata (metadata is an object field)

**Improved Solution:**
- Use CUE's `IncompleteKind()` to detect if a schema is structurally a list type
- Added `IsListType bool` field to `SchemaMetadata` (derived from CUE structure, not metadata)
- Updated filtering to use `meta.IsListType` instead of checking `schema_type` metadata

**Files Modified:**
- `internal/validator/validation_result.go` - Added `IsListType` to SchemaMetadata
- `internal/validator/cue_loader.go` - Detect list types using `schemaValue.IncompleteKind() & cue.ListKind`
- `internal/inference/heuristics.go` - Use `meta.IsListType` for collection filtering
- `internal/inference/inference.go` - Use `meta.IsListType` in findFallbackSchema

### Files Modified
- `internal/inference/heuristics.go` - Added CollectionType to hints, filtering logic
- `internal/inference/inference.go` - Added findFallbackSchema function
- `cmd/schema.go` - Pass collection type in reinfer command

---

## Feature: Smart Collection Schema Generation (pudl-0gi)

### Problem
When running `pudl schema new --from <collection> --collection`, the command generated an object schema with `schema_type: "collection"` in metadata. This is incorrect - collection schemas should be structurally arrays (list types).

### Solution
Implemented smart collection schema generation that:
1. Runs schema inference on each item in the collection
2. Groups items by their inferred schema (reuses existing schemas where possible)
3. Generates new item schemas only for items that don't match any existing schema
4. Creates a proper list-type collection schema as a union of all item types

### Example Output
```cue
package ec2

import (
	single "pudl.schemas/test/single"
)

#Ec2InstanceCollection: [...(#Ec2Instance | #Instance | single.#Item)]
```

### Key Features
- **Schema Reuse**: Automatically detects and reuses existing schemas that match items
- **Heterogeneous Collections**: Supports collections with mixed item types via union types
- **Proper Imports**: Generates CUE import statements for cross-package references
- **New Schema Generation**: Creates new item schemas for unmatched items

### Files Modified
- `internal/schemagen/generator.go`:
  - Added `CollectionGenerateOptions` and `CollectionGenerateResult` types
  - Added `GenerateSmartCollection()` method
  - Added `generateItemSchemaForUnmatched()` helper
  - Added `generateCollectionListSchema()` with proper import handling
  - Added `parseSchemaRef()` for cross-package reference parsing
- `cmd/schema.go`:
  - Added `runSmartCollectionGeneration()` function
  - Modified `runSchemaNewCommand()` to use smart generation for collections

### Public API
```
pudl schema new --from <collection-proquint> --path <package>:#<CollectionName> --collection
```

When `--collection` is used with a collection entry:
- Infers schemas for each item
- Reuses existing schemas where confidence >= 0.5
- Generates new item schemas for unmatched items
- Creates list-type collection schema with union of all item types

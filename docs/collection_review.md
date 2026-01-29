# Collection-Aware Schema Review Flow

## Analysis & Incremental Improvement Plan

### Current State Summary

The system already has the **metadata infrastructure** for collections (`CollectionID`, `ItemIndex`, `CollectionType` in CatalogEntry), but the **review flow UI and logic** treats everything as opaque `interface{}`. The schema creator generates field definitions without understanding "this is an array of items that share a common structure."

### The Core Problem

When `item.Data` is:
```json
[
  {"id": "1", "name": "foo", "status": "active"},
  {"id": "2", "name": "bar", "status": "pending"}
]
```

The review flow just shows this as "here's some JSON" and schema creation would generate something like:
```cue
#MySchema: {
  [0]: {...}  // or just treats it as opaque
}
```

Instead of recognizing: "this is `[]Item` where `Item` has `id`, `name`, `status`."

---

## Incremental Improvement Path

### Phase 1: Detection & Display (Low effort, high visibility)

**Goal**: Make the review flow *aware* and *communicate* when data is a collection.

**Changes in `interface.go`**:

```go
func analyzeDataShape(data interface{}) DataShape {
    switch v := data.(type) {
    case []interface{}:
        if len(v) == 0 {
            return DataShape{Type: "empty_array"}
        }
        // Check if homogeneous
        if isHomogeneous(v) {
            return DataShape{
                Type: "homogeneous_array",
                ItemCount: len(v),
                SampleItem: v[0],
                CommonFields: extractCommonFields(v),
            }
        }
        return DataShape{Type: "heterogeneous_array", ItemCount: len(v)}
    case map[string]interface{}:
        return DataShape{Type: "object", Fields: extractFields(v)}
    default:
        return DataShape{Type: "scalar"}
    }
}
```

**UI Enhancement** in `showDataPreview()`:

```
📦 Data Preview:
   Type: Array of 47 items (homogeneous)
   Common fields: id, name, status, created_at

   Sample item:
   {"id": "1", "name": "foo", "status": "active", ...}
```

This immediately gives the user context about what they're looking at.

---

### Phase 2: Schema for Items (Medium effort, significant value)

**Goal**: When creating a schema for a homogeneous collection, offer to create a schema for the *item type*.

**New prompt option** during schema creation:

```
This appears to be a collection of 47 similar items.
How would you like to define the schema?

  [1] Schema for the whole collection (array type)
  [2] Schema for individual items (recommended for homogeneous data)
  [3] Both (collection wrapper + item schema)
```

**Implementation approach**:

1. Add a `CollectionSchemaMode` enum: `Whole`, `ItemOnly`, `Both`
2. Modify `schema_creator.go:generateCUETemplate()` to accept this mode
3. For `ItemOnly` mode, extract a representative item and generate schema for that
4. Template becomes:

```cue
package mypackage

// #Item defines the schema for individual items
#Item: {
    id: string
    name: string
    status: "active" | "pending" | "inactive"
}

// Optional: collection constraint
#ItemCollection: [...#Item]
```

This is probably the **highest value incremental change** - it addresses the most common case (homogeneous arrays like AWS resource lists).

---

### Phase 3: Heterogeneous Collection Handling (Higher effort)

**Goal**: Support collections with mixed item types.

**Approach A - Discriminator Field**:
If items have a type/kind field, use it:

```
Detected discriminator field: "resourceType"
Values found: "ec2:instance" (23), "s3:bucket" (15), "rds:instance" (9)

Values found: "ec2:instance" (23), "s3:bucket" (15), "rds:instance" (9)

Would you like to:
  [1] Create a single union schema
  [2] Create separate schemas per type
  [3] Create base schema + type-specific extensions
```

**Approach B - Structural Clustering**:
Group items by their field signatures:

```
Found 2 structural patterns:
  Pattern A (35 items): id, name, vpcId, subnetId, ...
  Pattern B (12 items): id, bucketName, region, ...
```

This is more complex but handles real-world heterogeneous exports.

---

### Phase 4: Full Collection + Item Schema (Long-term)

**Goal**: Support the pattern:
- Collection has metadata (source, timestamp, query params)
- Collection contains items
- Items have their own schema

```cue
#ResourceExport: {
    exportTime: string
    source: string
    items: [...#Resource]
}

#Resource: {
    id: string
    // ...
}
```

This requires understanding collection structure at import time, which the JSON processor already partially does.

---

## Recommended Starting Point

**Phase 1 + Phase 2 together** as the first increment:

1. **Detection** (`analyzeDataShape`) - ~50-100 lines in a new file or added to `interface.go`
2. **Enhanced preview** - ~30 lines modifying `showDataPreview()`
3. **Schema mode prompt** - ~50 lines in `schema_creator.go`
4. **Item schema generation** - ~100 lines modifying `generateCUETemplate()`

**Why this grouping**: Detection without action is unsatisfying UX. Users will see "this is a collection" but can't do anything about it. Combining detection with the ability to create item schemas delivers immediate value.

---

## Questions to Consider

Before diving in:

1. **Should item schemas be automatically suggested?** When we detect homogeneous arrays, should we default to "create item schema" or always ask?

2. **What about nested collections?** An item might contain arrays (e.g., `securityGroups: [...]`). How deep should we go?

3. **Schema naming convention**: If data is `instances.json`, should item schema be `#Instance` (singular) automatically?

4. **Validation mode**: When validating a collection against an item schema, should we validate each item individually and report per-item errors?

---

## Key Files

| File | Purpose |
|------|---------|
| `internal/review/interface.go` | Main interactive review loop, user prompts, actions |
| `internal/review/schema_creator.go` | Schema generation from data, template processing |
| `internal/review/fetcher.go` | Loads catalog items, prepares for review, schema suggestion |
| `internal/review/session.go` | Session management, state persistence, progress tracking |
| `internal/database/catalog.go` | Catalog entry structure with collection support |
| `internal/streaming/json_processor.go` | JSON/array parsing from chunks |

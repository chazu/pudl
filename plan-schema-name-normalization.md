# Schema Name Normalization Implementation Plan

## Overview

**Goal:** Normalize schema names throughout the PUDL codebase to use a canonical format `<pkg>.#Name` (e.g., `aws/ec2.#Instance`, `pudl/core.#Item`), making the `@v0` version suffix internal/optional.

**Problem:** Currently, schema names appear in multiple formats across the codebase:
- CUE loader creates: `pudl.schemas/aws/ec2@v0:#Instance`
- Some code uses: `core.#Item`
- Other code uses: `aws/ec2.#Instance`
- CLI accepts: `aws/ec2:#Instance` or `aws/ec2.#Instance`

**Success Criteria:**
1. All user-facing schema names use canonical format: `<pkg>.#Name`
2. Internal CUE module resolution handles `@v0` suffix transparently
3. Lookups work regardless of input format (normalized before comparison)
4. Database stores canonical format
5. Existing data migrates seamlessly

---

## Prerequisites

- Understanding of CUE module naming (`pudl.schemas@v0`)
- Access to modify validator, inference, database, and CLI components
- Test data with existing schema assignments in the database

---

## Implementation Steps

### Step 1: Create Schema Name Normalization Package

**Description:** Create a new package `internal/schemaname` with utility functions for parsing, normalizing, and formatting schema names.

**Files to create:**
- `internal/schemaname/schemaname.go`

**Key Functions:**

```go
package schemaname

// Canonical format: "aws/ec2.#Instance"
// CUE internal format: "pudl.schemas/aws/ec2@v0:#Instance"

// Normalize converts any schema name format to canonical format
func Normalize(name string) string

// ToCUEInternal converts canonical to CUE module format for lookups
func ToCUEInternal(canonical string) string

// Parse extracts package path and definition from any format
func Parse(name string) (pkgPath string, definition string, err error)

// Format creates canonical format from components
func Format(pkgPath, definition string) string

// IsEquivalent checks if two schema names refer to the same schema
func IsEquivalent(a, b string) bool
```

**Implementation details:**

```go
// Normalize converts any schema name format to canonical format
// Input examples:
//   "pudl.schemas/aws/ec2@v0:#Instance" → "aws/ec2.#Instance"
//   "pudl.schemas/aws/ec2:#Instance"    → "aws/ec2.#Instance"
//   "aws/ec2:#Instance"                 → "aws/ec2.#Instance"
//   "aws/ec2.#Instance"                 → "aws/ec2.#Instance"
//   "core.#Item"                        → "pudl/core.#Instance"
func Normalize(name string) string {
    // 1. Strip "pudl.schemas/" prefix if present
    // 2. Strip "@v0" (or any @version) suffix
    // 3. Convert ":" separator to "."
    // 4. Handle legacy short names (core.#Item -> pudl/core.#Item)
}
```

**Testing considerations:**
- Unit tests for all format variations
- Edge cases: empty strings, malformed names, multiple @ symbols

---

### Step 2: Update CUE Loader to Store Canonical Names

**Description:** Modify `internal/validator/cue_loader.go` to use canonical format when creating schema names, while keeping CUE internal format for module resolution.

**Files to modify:**
- `internal/validator/cue_loader.go`

**Changes in `createModuleFromInstance`:**

```go
// Line ~140: Change schema name generation
// FROM:
schemaName := fmt.Sprintf("pudl.schemas/%s:%s", moduleName, label)

// TO:
canonicalName := schemaname.Format(moduleName, label)
schemas[canonicalName] = schemaValue

// Also store a lookup map for CUE internal names -> canonical
```

**Keep internal CUE format for:**
- Module loading and cross-reference resolution
- CUE value lookups (use a reverse mapping)

---

### Step 3: Add Lookup Resolution in CascadeValidator

**Description:** Update `internal/validator/cascade_validator.go` to normalize schema names before lookups.

**Files to modify:**
- `internal/validator/cascade_validator.go`

**Changes:**

```go
// Update findFallbackSchemaName to use normalized lookups
func (cv *CascadeValidator) findFallbackSchemaName() string {
    // Use schemaname.Normalize for all patterns
    fallbackPatterns := []string{
        "pudl/core.#Item",  // Canonical format
    }
    // ...
}

// Update ResolveSchemaName to normalize input
func (cv *CascadeValidator) ResolveSchemaName(userInput string) (string, error) {
    normalized := schemaname.Normalize(userInput)
    // ...
}

// Update ValidateWithCascade to normalize intendedSchema
func (cv *CascadeValidator) ValidateWithCascade(data interface{}, intendedSchema string) (*ValidationResult, error) {
    intendedSchema = schemaname.Normalize(intendedSchema)
    // ...
}
```

---

### Step 4: Update Inference Engine

**Description:** Modify `internal/inference/inference.go` to use canonical format.

**Files to modify:**
- `internal/inference/inference.go`

**Changes:**

```go
// Update isCatchallSchema to use canonical format
func isCatchallSchema(schemaName string) bool {
    normalized := schemaname.Normalize(schemaName)
    return normalized == "pudl/core.#Item" ||
           normalized == "pudl/core.#CatchAll"
}

// Update findCatchallSchema
func findCatchallSchema(schemas map[string]cue.Value) string {
    canonical := "pudl/core.#Item"
    if _, exists := schemas[canonical]; exists {
        return canonical
    }
    // Fallback search...
}

// Update findFallbackSchema similarly
```

---

### Step 5: Update Database Catalog

**Description:** The catalog stores schema names. Ensure all new entries use canonical format and provide migration for existing data.

**Files to modify:**
- `internal/database/catalog.go`

**Changes:**

1. **Normalize on insert:**
```go
func (c *CatalogDB) AddEntry(entry CatalogEntry) error {
    // Normalize schema before storing
    entry.Schema = schemaname.Normalize(entry.Schema)
    // ... rest of insert
}
```

2. **Normalize on query filters:**
```go
func (c *CatalogDB) QueryEntries(filters FilterOptions, options QueryOptions) (*QueryResult, error) {
    if filters.Schema != "" {
        filters.Schema = schemaname.Normalize(filters.Schema)
    }
    // ... rest of query
}
```

3. **Add migration function:**
```go
// MigrateSchemaNames normalizes all existing schema names in the database
func (c *CatalogDB) MigrateSchemaNames() (int, error) {
    // SELECT all entries, normalize schema, UPDATE if different
}
```

---

### Step 6: Update Importer

**Description:** Ensure importer assigns normalized schema names.

**Files to modify:**
- `internal/importer/importer.go`

**Changes:**

```go
// In ImportFile, after schema inference:
schema = schemaname.Normalize(schema)

// In createCollectionEntry:
schema := schemaname.Normalize("pudl/core.#Collection")

// In assignItemSchema:
fallback := schemaname.Normalize("pudl/core.#Item")
```

---

### Step 7: Update CLI Commands

**Description:** Update CLI to display and accept normalized format.

**Files to modify:**
- `cmd/schema.go`

**Changes:**

1. **Update parseSchemaPath:** Already handles `:#` format, add normalization:
```go
func parseSchemaPath(path string) (packagePath, definitionName string) {
    // Existing parsing logic...
    // No change needed - already produces clean paths
}
```

2. **Update schema display commands:**
```go
// In listAllSchemas, use canonical format:
fmt.Printf("   ├─ %s\n", schemaInfo.FullName) // Already correct format

// Update schema show command to normalize input:
func runSchemaShowCommand(schemaArg string) error {
    // Normalize the input before parsing
    normalized := schemaname.Normalize(schemaArg)
    // ...
}
```

3. **Update completion functions to return canonical format**

---

### Step 8: Update Schema Generator

**Description:** Ensure generated schemas use canonical format in references.

**Files to modify:**
- `internal/schemagen/generator.go`

**Changes:**

```go
// In parseSchemaRef, use normalization:
func parseSchemaRef(ref string, currentPackage string) struct { ... } {
    // Normalize the reference first
    ref = schemaname.Normalize(ref)
    // ... rest of parsing
}

// Update generateCollectionListSchema to use canonical refs
```

---

### Step 9: Update Review System

**Description:** Ensure review workflow uses canonical format.

**Files to modify:**
- `internal/review/schema_creator.go`
- `internal/review/catalog_updater.go`

**Changes:**
- Normalize schema names when creating new schemas
- Normalize when updating catalog entries

---

## File Changes Summary

### Files to Create:
| File | Purpose |
|------|---------|
| `internal/schemaname/schemaname.go` | Core normalization functions |
| `internal/schemaname/schemaname_test.go` | Unit tests |

### Files to Modify:
| File | Changes |
|------|---------|
| `internal/validator/cue_loader.go` | Use canonical format for schema names |
| `internal/validator/cascade_validator.go` | Normalize lookups and resolution |
| `internal/inference/inference.go` | Normalize catchall checks |
| `internal/inference/heuristics.go` | No changes needed (uses metadata) |
| `internal/database/catalog.go` | Normalize on insert/query + migration |
| `internal/importer/importer.go` | Normalize assigned schemas |
| `cmd/schema.go` | Normalize user input in commands |
| `internal/schemagen/generator.go` | Normalize schema references |
| `internal/review/schema_creator.go` | Normalize created schema names |
| `internal/review/catalog_updater.go` | Normalize updated schema names |
| `internal/streaming/cue_integration.go` | Update matchesSchemaName |

---

## Testing Strategy

### Unit Tests

1. **schemaname package tests:**
   - Test Normalize with all known input formats
   - Test Parse with valid and invalid inputs
   - Test IsEquivalent with various combinations
   - Test Format produces correct output

2. **Integration tests for each modified component**

### Integration Tests

1. **Import flow:** Import data, verify canonical schema in catalog
2. **Inference flow:** Verify returned schemas are canonical
3. **Query flow:** Search by various formats, find same entries
4. **CLI flow:** Use different input formats, get consistent output

### Manual Testing Steps

1. Import a JSON file, check `pudl ls` shows canonical schema format
2. Run `pudl schema show aws/ec2.#Instance` and `pudl schema show pudl.schemas/aws/ec2@v0:#Instance` - both should work
3. Run `pudl schema reinfer --schema pudl/core.#Item` - should find entries
4. Check existing tests pass after changes

---

## Rollback Plan

1. **Revert code changes:** Git revert the PR
2. **Database rollback:** Keep original schema values in a backup column during migration, restore if needed:
   ```sql
   ALTER TABLE catalog_entries ADD COLUMN schema_backup TEXT;
   UPDATE catalog_entries SET schema_backup = schema;
   -- After migration:
   UPDATE catalog_entries SET schema = schema_backup;
   ```

---

## Estimated Effort

| Task | Effort | Complexity |
|------|--------|------------|
| Step 1: Create schemaname package | 2 hours | Low |
| Step 2: Update CUE loader | 1 hour | Medium |
| Step 3: Update CascadeValidator | 1 hour | Medium |
| Step 4: Update Inference | 1 hour | Low |
| Step 5: Update Database | 2 hours | Medium |
| Step 6: Update Importer | 1 hour | Low |
| Step 7: Update CLI | 1 hour | Low |
| Step 8: Update Schema Generator | 1 hour | Low |
| Step 9: Update Review System | 0.5 hour | Low |
| Testing | 2 hours | Medium |
| **Total** | **~12 hours** | **Medium** |

---

## Implementation Order

Recommended order to minimize risk:

1. **Step 1:** Create schemaname package (foundation)
2. **Step 2:** Update CUE loader (source of truth)
3. **Step 3-4:** Update validator and inference (consumers)
4. **Step 6:** Update importer (new data entry point)
5. **Step 5:** Update database with migration
6. **Step 7-9:** Update CLI and generators
7. **Testing:** Full test suite

---

## Appendix: Detailed Format Specifications

### Canonical Format
```
<package-path>.#<DefinitionName>

Examples:
- aws/ec2.#Instance
- pudl/core.#Item
- k8s/v1.#Pod
```

### CUE Internal Format (for module loading)
```
pudl.schemas/<package-path>@v0:#<DefinitionName>

Examples:
- pudl.schemas/aws/ec2@v0:#Instance
- pudl.schemas/pudl/core@v0:#Item
```

### Mapping Table

| CUE Internal | Canonical | Legacy Short |
|--------------|-----------|--------------|
| `pudl.schemas/aws/ec2@v0:#Instance` | `aws/ec2.#Instance` | - |
| `pudl.schemas/pudl/core@v0:#Item` | `pudl/core.#Item` | `core.#Item` |
| `pudl.schemas/k8s/v1@v0:#Pod` | `k8s/v1.#Pod` | - |


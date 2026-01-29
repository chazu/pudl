# ID Generation Refactoring

**Date:** 2025-01-16
**Status:** Completed

## Summary

Refactored the ID generation system from a complex multi-format approach to a simple, deterministic content-based hashing system with proquint display format.

## What Changed

### Removed
- **5 ID formats**: short code, readable, compact, sequential formats - kept only proquint
- **Complex ID manager**: `ImporterIDManager`, `IDDisplayHelper`, context-aware selection
- **ID configuration system**: `internal/config/id_config.go` (239 lines)
- **Demo program**: `cmd/demo-ids/` (203 lines)
- **Old implementation files**:
  - `internal/idgen/friendly_ids.go` (352 lines)
  - `internal/idgen/integration.go` (234 lines)
  - `internal/idgen/friendly_ids_test.go` (297 lines)

**Total lines removed:** ~1,325 lines

### Added
- **Content hashing**: `internal/idgen/content_hash.go` (165 lines)
  - `ComputeContentID(data []byte) string` - SHA256 hash for storage
  - `HashToProquint(hashHex string) string` - Convert hash to proquint for display
  - `GenerateRandomProquint()` - For cases where content hashing isn't applicable
  - Proquint conversion functions (number ↔ proquint)

- **Comprehensive tests**: `internal/idgen/content_hash_test.go` (220 lines)
  - Content ID determinism tests
  - Proquint conversion tests
  - Round-trip conversion tests
  - Complete workflow tests

**Total lines added:** ~385 lines

**Net reduction:** ~940 lines (72% reduction in ID generation code)

## Design Decisions

### Why Content Hashing?
- **Deduplication**: Same data always produces same ID
- **Idempotency**: Re-importing identical files won't create duplicates
- **Reproducibility**: Anyone importing same data gets same ID
- **Industry standard**: Used by Git, S3, content-addressable storage systems

### Why Store Full Hash, Display as Proquint?
- **Storage**: Full SHA256 hash (64 hex chars) provides complete benefits
- **Display**: Proquint (11 chars: "lusab-babad") is human-friendly for CLI/logs
- **Conversion**: Deterministic - same hash always maps to same proquint prefix
- **Flexibility**: Can show full hash when needed for debugging

### Collection Items
- Each item's ID is computed from the item's content hash
- Deterministic - same item data = same ID across imports
- Enables deduplication at the item level

## API

### Public Functions

```go
// Storage - use full hash
id := idgen.ComputeContentID(fileData)  // Returns 64-char SHA256 hex string

// Display - convert to proquint
displayID := idgen.HashToProquint(id)   // Returns 11-char proquint

// Random proquint (for non-content-based needs)
randomID := idgen.GenerateRandomProquint()

// Low-level proquint conversion
proquint := idgen.NumberToProquint(uint32(12345))
number, err := idgen.ProquintToNumber("lusab-babad")
```

### Enhanced Importer Changes

```go
// Before
idManager := idgen.NewImporterIDManagerFromOrigin(origin)
mainID := idManager.GenerateMainID(sourcePath, origin)

// After
fileData, _ := os.ReadFile(sourcePath)
mainID := idgen.ComputeContentID(fileData)
```

## Migration Impact

- **Backward compatibility**: None needed - this is a prototype
- **Existing data**: IDs will change on re-import (content-based now)
- **Deduplication**: New system automatically prevents duplicate imports

## Benefits

1. **Simplicity**: One ID strategy instead of five
2. **Determinism**: Same data always = same ID
3. **Deduplication**: Automatic via content addressing
4. **Human-friendly**: Proquints for display ("lusab-babad")
5. **Standard**: Follows industry best practices (Git, S3)
6. **Maintainability**: 72% less code to maintain

## Testing

All tests passing:
- ✅ Content ID computation (determinism)
- ✅ Hash to proquint conversion
- ✅ Random proquint generation
- ✅ Round-trip conversions
- ✅ Importer integration tests
- ✅ System tests

## Files Modified

- `internal/idgen/content_hash.go` (new)
- `internal/idgen/content_hash_test.go` (new)
- `internal/importer/enhanced_importer.go` (simplified)
- Deleted: `internal/config/id_config.go`, `cmd/demo-ids/`, old idgen files
- Removed: `docs/human-friendly-ids.md`

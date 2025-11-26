# CUE Loader Debug Fix & Duplicate Import Handling

## Date: 2025-11-25

## Problems Fixed

### Problem 1: CUE Loader Failure
The `pudl import` command was failing with "Failed to initialize enhanced importer" error. The root cause was the CUE module loader attempting to load all packages including `examples/kubernetes.cue` which imports third-party CUE modules (`cue.dev/x/k8s.io/api/apps/v1` and `cue.dev/x/k8s.io/api/core/v1`) that were not fetched.

### Problem 2: Duplicate Import Errors
Importing the same file twice would fail with a database constraint error: `UNIQUE constraint failed: catalog_entries.id`. The system should skip duplicates gracefully instead.

## Solutions

### 1. Added Verbose Logging to CUE Loader
Added logging infrastructure to `internal/validator/cue_loader.go`:
- `SetVerbose(bool)` method to enable/disable verbose logging
- `log(format string, args ...interface{})` helper function
- Log messages throughout `LoadAllModules()` for debugging

### 2. Fixed CUE Loader to Skip Examples Directory
Modified `LoadAllModules()` in `internal/validator/cue_loader.go` to skip the examples directory which may contain schemas with unfetched third-party dependencies:
```go
// Skip examples directory - it may have unfetched third-party dependencies
if strings.Contains(inst.Dir, "/examples") || inst.PkgName == "examples" {
    loader.log("Skipping examples directory: %s", inst.Dir)
    continue
}
```

### 3. Added Debug Mode to Import Command
Added `PUDL_DEBUG=1` environment variable support to `cmd/import.go` to print detailed errors during import operations.

### 4. Added Duplicate Detection
- Added `EntryExists(id string)` method to `internal/database/catalog.go`
- Added `Skipped` and `SkipReason` fields to `ImportResult` struct
- Modified `ImportFileWithFriendlyIDs()` to check for existing content before importing
- Updated `displayImportResults()` to show skip message for duplicates

## Files Modified
- `internal/validator/cue_loader.go` - Added verbose logging and examples directory skip
- `internal/database/catalog.go` - Added `EntryExists()` method
- `internal/importer/importer.go` - Added `Skipped` and `SkipReason` fields to `ImportResult`
- `internal/importer/enhanced_importer.go` - Added duplicate detection before import
- `cmd/import.go` - Added PUDL_DEBUG support and skip display handling

## Public API

### New Database Method
```go
// EntryExists checks if a catalog entry with the given ID exists
func (c *CatalogDB) EntryExists(id string) (bool, error)
```

### Updated ImportResult
```go
type ImportResult struct {
    // ... existing fields ...
    Skipped    bool   `json:"skipped,omitempty"`
    SkipReason string `json:"skip_reason,omitempty"`
}
```

## Testing
- All `internal/inference/...` tests pass
- All `internal/importer/...` tests pass
- All `internal/database/...` tests pass
- Manual testing confirms:
  - New files import correctly
  - Duplicate files are skipped with informative message

## Usage
```bash
# Normal import
pudl import --path data.json

# Debug mode for troubleshooting
PUDL_DEBUG=1 pudl import --path data.json

# Duplicate import (gracefully skipped)
pudl import --path data.json  # If already imported
# Output: ⏭️  Skipped: data.json
#         Reason: content already exists in catalog
```

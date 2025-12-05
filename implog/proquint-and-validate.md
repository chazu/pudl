# Proquint IDs and Validate Command Implementation

## Summary

This implementation adds two major features:
1. Proquint IDs as the standard human-friendly identifier displayed across the CLI
2. A new `pudl validate` command to validate catalog entries against their assigned schemas

## Changes Made

### 1. Proquint ID Display (System-wide)

Proquint identifiers are now the primary way entry IDs are displayed throughout the CLI. Proquints are derived from the first 32 bits of the SHA256 content hash, providing a human-readable identifier like `babod-fakak` instead of a 64-character hex string.

**Files Modified:**

- `internal/lister/lister.go`
  - Added `Proquint` field to `ListEntry` struct
  - Updated `ListData()` and `FindEntry()` to populate proquint from hash
  - `FindEntry()` now accepts both proquint and full hash IDs

- `internal/database/catalog.go`
  - Added `GetEntryByProquint(proquint string)` method
  - Converts proquint to hex prefix and queries with LIKE
  - Handles collision detection (returns error if multiple matches)

- `cmd/list.go`
  - Updated `displayEntry()` to show proquint as primary ID
  - Verbose mode shows full hash under "Hash:" label

- `cmd/show.go`
  - Updated display to show "Entry:" with proquint
  - Added "Hash:" line for full content hash

- `internal/ui/types.go`
  - Updated `Title()` to display proquint
  - Updated `FilterValue()` to include proquint in search

- `internal/ui/list.go`
  - Updated `formatDetailedEntry()` for proquint display

### 2. Validate Command

New command to validate catalog entries against their assigned CUE schemas.

**File Created:**

- `cmd/validate.go`

**Public API:**

```bash
pudl validate --entry <proquint>   # Validate single entry by proquint ID
pudl validate --all                # Validate all catalog entries
```

**Features:**
- Validates data against assigned schema using CUE cascade validation
- Shows detailed validation report for single entry mode
- Summary statistics for all-entries mode
- Lists invalid entries with error details
- Exit code indicates validation success/failure

### 3. Database API Addition

**New Method in `internal/database/catalog.go`:**

```go
// GetEntryByProquint retrieves an entry by its proquint identifier
// Proquints are derived from the first 32 bits of the content hash
func (c *CatalogDB) GetEntryByProquint(proquint string) (*CatalogEntry, error)
```

This method:
- Converts proquint to 8-character hex prefix
- Queries using `WHERE id LIKE prefix%`
- Returns error if ambiguous (multiple matches on prefix)
- Returns ErrCodeNotFound if no match

## Testing

- All existing tests pass
- Build succeeds with no errors
- Validate command help text displays correctly

## Usage Examples

```bash
# List entries - now shows proquint IDs
$ pudl list
1. babod-fakak [aws.#EC2Instance] (2024-12-04T10:30:00Z)
   Origin: aws-ec2 | Format: json | Records: 1 | Size: 1.2 KB

# Show entry by proquint
$ pudl show babod-fakak
Entry: babod-fakak
Hash: a1b2c3d4e5f6...
Schema: aws.#EC2Instance
...

# Validate single entry
$ pudl validate --entry babod-fakak

# Validate all entries
$ pudl validate --all
Validating 47 entries...

  babod-fakak [aws.#EC2Instance] - VALID
  ...

Validation Summary
═══════════════════════════════════════════════════════════════
Total entries: 47
Valid:         45
Invalid:       2
Errors:        0
```

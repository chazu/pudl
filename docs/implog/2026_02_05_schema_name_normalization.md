# Schema Name Normalization

**Date:** 2026-02-05

## Summary

Implemented canonical schema name normalization to standardize schema references throughout the pudl codebase. This replaces various internal CUE formats with a consistent `<pkg>.#Name` format.

## Public API

### New Package: `internal/schemaname`

```go
// Normalize converts any schema name format to canonical format
func Normalize(name string) string

// Parse extracts package path and definition from any schema name format
func Parse(name string) (pkgPath, definition string, ok bool)

// Format creates a canonical schema name from package and definition
func Format(pkg, def string) string

// IsEquivalent compares two schema names for equivalence after normalization
func IsEquivalent(a, b string) bool

// IsFallbackSchema checks if the schema name refers to a fallback/catchall schema
func IsFallbackSchema(name string) bool
```

### New CLI Command

```bash
pudl schema migrate  # Migrates existing schema names to canonical format
```

### Database Methods

```go
// MigrateSchemaNames normalizes all schema names in the catalog
func (db *CatalogDB) MigrateSchemaNames() (int, error)
```

## Canonical Format

**Format:** `<pkg>.#Name`

Examples:
- `aws/ec2.#Instance`
- `aws/ec2.#InstanceCollection`
- `aws/eks.#Cluster`
- `pudl/core.#Item`
- `pudl/core.#Collection`

This replaces various formats:
- `pudl.schemas/aws/ec2@v0:#Instance`
- `aws/ec2@v0:#Instance`
- `aws/ec2:#Instance`
- `core.#Item` (legacy short format)

## Files Changed

- `internal/schemaname/schemaname.go` - New package with normalization utilities
- `internal/schemaname/schemaname_test.go` - Comprehensive tests
- `internal/validator/cue_loader.go` - Use canonical format for schema names
- `internal/validator/cascade_validator.go` - Normalize schema lookups
- `internal/inference/inference.go` - Use schemaname.IsFallbackSchema()
- `internal/database/catalog.go` - Normalize on AddEntry/UpdateEntry
- `internal/database/testutil.go` - Use IsEquivalent() for schema comparison
- `internal/inference/inference_test.go` - Remove obsolete containsCatchAll test
- `cmd/schema.go` - Add migrate subcommand

## Testing

All tests pass. The schemaname package has comprehensive tests covering:
- Normalization of all formats
- Parsing and formatting
- Equivalence checking
- Fallback schema detection (including CatchAll variants)


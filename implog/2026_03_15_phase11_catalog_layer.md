# Phase 11: Schema Catalog Layer

**Date:** 2026-03-15

## Summary

Added a central catalog layer that registers pudl's schema types as a browsable inventory. The catalog follows the same bootstrap embedding pattern as `core.cue` and provides a `pudl catalog` CLI command.

## Files Created

| File | Purpose |
|------|---------|
| `internal/importer/bootstrap/pudl/catalog/catalog.cue` | Bootstrap CUE catalog with `#CatalogEntry` definition and core type registrations |
| `cmd/catalog.go` | `pudl catalog` command implementation |

## Files Modified

| File | Change |
|------|--------|
| `internal/importer/cue_schemas.go` | Added catalog schema check to `ensureBasicSchemas()` |
| `docs/plan.md` | Added Phase 11 entry and completion log |

## Public API

### CUE Types

- `#CatalogEntry` — Schema type registration with fields: `schema`, `schema_type`, `resource_type`, `description`
- `entries` — Map of canonical schema name to `#CatalogEntry`

### CLI Commands

- `pudl catalog` — Lists all registered schema types with their metadata
- `pudl catalog --verbose` — Shows additional metadata fields (identity_fields, tracked_fields, list_type)

## Design Decisions

- The catalog uses CUE's `_pudl` metadata annotations already present on schemas rather than requiring a separate registration step
- The `catalog.cue` file serves as the canonical definition of catalog structure; actual type discovery comes from loading all CUE modules at runtime
- Auto-bootstrap in `ensureBasicSchemas()` ensures the catalog schema is copied alongside core schemas

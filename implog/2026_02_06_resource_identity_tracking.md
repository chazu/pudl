# Resource Identity Tracking

**Date:** 2026-02-06

## Summary

Added resource identity tracking to PUDL so the system can recognize "the same resource across imports." Every catalog entry now gets a stable `resource_id` derived from its schema's identity fields. When the same resource is imported again with different data, it becomes a new version. When exact content is reimported, it's a duplicate and skipped.

## Design Decisions

- **Every entry has an identity:** schemas with `identity_fields` use extracted field values; catchall schemas (empty `identity_fields`) use content hash as identity.
- **Import decision tree:** no match → new resource. Same identity + same hash → skip. Same identity + different hash → new version.
- **Collections are provenance, not identity.** Resources own their identity independent of collection.
- **Catchall entries are effectively immutable** — identity = hash means changed content = different resource.
- **Content hash is the universal dedup gate** — if hash matches any existing entry, skip regardless of schema.
- **Reinfer recomputes identity** when schema changes (version stays the same).

## New Package: `internal/identity/`

### `extract.go`
- `ExtractFieldValues(data interface{}, fields []string) (map[string]interface{}, error)` — extracts values from parsed JSON for given field paths (dot-notation supported)
- `extractNestedField(data interface{}, path string) (interface{}, bool)` — traverses nested maps

### `resource_id.go`
- `ComputeResourceID(schema string, identityValues map[string]interface{}, contentHash string) string` — deterministic SHA256 resource ID
- `CanonicalIdentityJSON(values map[string]interface{}) (string, error)` — sorted-key JSON for determinism

## Database Changes: `internal/database/`

### New Columns on `catalog_entries`
- `resource_id TEXT` — deterministic hash of (schema, identity)
- `content_hash TEXT` — SHA256 of raw stored data
- `identity_json TEXT` — canonical JSON of identity field values
- `version INTEGER DEFAULT 1` — monotonic per resource_id

### New Indexes
- `idx_resource_id`, `idx_content_hash`, `idx_resource_version`

### New Files
- `catalog_migrations.go` — idempotent column migration + backfill
- `catalog_identity.go` — `FindByContentHash`, `FindByResourceID`, `GetLatestVersion`

### Modified: `catalog.go`
- `CatalogEntry` struct extended with identity fields
- `AddEntry` / `UpdateEntry` SQL includes new columns
- All scan operations (`GetEntry`, `GetEntryByProquint`, `QueryEntries`, `GetCollectionItems`, `GetCollectionByID`) updated

## Import Flow Changes: `internal/importer/`

### `enhanced_importer.go`
- Replaced `EntryExists(mainID)` with `FindByContentHash(contentHash)` for dedup
- After schema inference, extracts identity fields via `inferrer.GetSchemaMetadata()`
- Computes `resource_id`, `identity_json`, and `version` before catalog insert
- Collection items also get identity tracking

### `importer.go`
- `ImportResult` extended with `ResourceID`, `ContentHash`, `Version`, `IsNewVersion`

### `metadata.go`
- `ResourceTracking` extended with `ResourceID`, `ContentHash`, `IdentityValues`, `Version`

## Reinfer Integration: `cmd/schema_reinfer.go`
- `reinferSingleEntry` and `applyReinferChanges` now recompute identity when schema changes
- Added `recomputeEntryIdentity` helper

## CLI Changes

### `cmd/list.go`
- Version shown next to proquint in list output (e.g., `babod-fakak v2`)
- Verbose mode shows Resource ID, Content Hash, Version

### `internal/lister/lister.go`
- `ListEntry` extended with `ResourceID`, `ContentHash`, `IdentityJSON`, `Version`
- Consolidated conversion via `dbEntryToListEntry` helper

### New Command: `pudl migrate identity [--dry-run]`
- `cmd/migrate.go` — top-level migrate command
- `cmd/identity_migrate.go` — backfills resource_id for entries imported before identity tracking

## Tests

- `internal/identity/extract_test.go` — 10 tests covering flat, nested, composite, missing, array, empty
- `internal/identity/resource_id_test.go` — 10 tests covering determinism, normalization, catchall, key ordering
- `internal/identity/integration_test.go` — 4 flow tests (same file twice, modified content, catchall, schema change)
- `internal/database/catalog_migrations_test.go` — 2 tests (idempotency, backfill)
- `internal/database/catalog_identity_test.go` — 4 tests (FindByContentHash, FindByResourceID, GetLatestVersion, full identity insert)

## Files

### New
- `internal/identity/extract.go`
- `internal/identity/extract_test.go`
- `internal/identity/resource_id.go`
- `internal/identity/resource_id_test.go`
- `internal/identity/integration_test.go`
- `internal/database/catalog_migrations.go`
- `internal/database/catalog_migrations_test.go`
- `internal/database/catalog_identity.go`
- `internal/database/catalog_identity_test.go`
- `cmd/migrate.go`
- `cmd/identity_migrate.go`

### Modified
- `internal/database/catalog.go`
- `internal/importer/enhanced_importer.go`
- `internal/importer/importer.go`
- `internal/importer/metadata.go`
- `internal/lister/lister.go`
- `cmd/list.go`
- `cmd/schema_reinfer.go`

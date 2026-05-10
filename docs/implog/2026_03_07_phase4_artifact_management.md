# Phase 4: Artifact Management — Unify Storage

**Date:** 2026-03-07

## Summary

Method outputs are now stored as catalog entries alongside imported data, using the same content-hashing, dedup, and SQLite catalog infrastructure. This enables querying execution history via `pudl data search` and `pudl data latest`.

## What Changed

### Database Migration (`internal/database/catalog_migrations.go`)
- Added `ensureArtifactColumns()` following the `ensureIdentityColumns()` pattern
- New columns: `entry_type`, `definition`, `method`, `run_id`, `tags`
- Indexes on `entry_type`, `definition`, `definition+method`, `run_id`
- Backfills `entry_type = 'import'` for existing rows

### Extended CatalogEntry (`internal/database/catalog.go`)
- 5 new nullable pointer fields on `CatalogEntry`: `EntryType`, `Definition`, `Method`, `RunID`, `Tags`
- `EntryType` filter added to `FilterOptions`
- All SELECT/Scan sites updated (8 in `catalog.go`, 2 in `catalog_identity.go`)
- `AddEntry` INSERT and `UpdateEntry` SET updated

### Artifact Query Methods (`internal/database/catalog_artifacts.go` — NEW)
- `GetLatestArtifact(definition, method)` — most recent artifact for a def+method pair
- `SearchArtifacts(filters)` — search artifacts with definition/method/limit filters
- `ArtifactFilters` struct for query parameters

### Artifact Storage Package (`internal/artifact/store.go` — NEW)
- `Store(db, opts)` — serialize output to JSON, content-hash, dedup check, write file + .meta sidecar, create CatalogEntry
- `StoreOptions` — definition, method, output, tags, data path
- `StoreResult` — ID, proquint, path, dedup flag
- Artifact path convention: `<dataPath>/artifacts/<def>/<method>/<ts>-<hash[:16]>.json`
- `run_id` = SHA256 of `definition|method|timestamp`

### Hook into `pudl method run` (`cmd/method_run.go`)
- After `exec.Run()` succeeds (non-dry-run): opens CatalogDB, calls `artifact.Store()`, prints proquint
- Non-fatal on failure (warnings printed to stderr)

### CLI `pudl data` Commands
- `cmd/data.go` — parent `pudl data` command
- `cmd/data_search.go` — `pudl data search [--definition X] [--method Y] [--limit N]`
- `cmd/data_latest.go` — `pudl data latest <definition> <method> [--raw]`

### Updated `pudl list` (`cmd/list.go`)
- `--artifacts` flag: show only artifacts
- `--all` flag: show both imports and artifacts
- Default behavior unchanged: imports only (backwards-compatible)
- Flags are mutually exclusive

### Updated Lister (`internal/lister/lister.go`)
- `EntryType` added to `FilterOptions`
- 5 artifact fields added to `ListEntry`
- `dbEntryToListEntry` maps new fields
- Filter passed through to database layer

## Public API

### `internal/artifact`
```go
func Store(db *database.CatalogDB, opts StoreOptions) (*StoreResult, error)

type StoreOptions struct {
    Definition string
    Method     string
    Output     interface{}
    Tags       map[string]string
    DataPath   string
}

type StoreResult struct {
    ID       string
    Proquint string
    Path     string
    Deduped  bool
}
```

### `internal/database`
```go
func (c *CatalogDB) GetLatestArtifact(definition, method string) (*CatalogEntry, error)
func (c *CatalogDB) SearchArtifacts(filters ArtifactFilters) ([]CatalogEntry, error)

type ArtifactFilters struct {
    Definition string
    Method     string
    Limit      int
}
```

### CLI Commands
- `pudl data search [--definition X] [--method Y] [--limit N]`
- `pudl data latest <definition> <method> [--raw]`
- `pudl list --artifacts` / `pudl list --all`

## Tests
- `internal/artifact/store_test.go` — basic store, dedup, tags (3 tests)
- `internal/database/catalog_artifacts_test.go` — latest, search, not-found, import exclusion (4 tests)
- All 291+ existing tests continue to pass

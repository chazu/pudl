# Fix `pudl list` entry_type filter taxonomy drift

## Symptom
`pudl list --all` showed entries, but copying a schema name and running
`pudl list --schema <x>` (or plain `pudl list`) returned
"No data found matching the specified criteria." Schema matching was a red
herring.

## Root cause
`cmd/list.go` defaulted the entry-type filter to the dead value `"import"`
(`--artifacts` → `"artifact"`, `--all` → no filter). The live ingestion
taxonomy is now `observe` (ingested/observed data, 1173 rows),
`manifest` + `manifest-action` (run outputs). Nothing writes `import`/`artifact`
anymore, so the default `entry_type='import'` filter matched zero rows. Only
`--all` (no filter) worked.

Writers of entry_type: `internal/mubridge/ingest.go` (`observe`),
`internal/mubridge/manifest.go` (`manifest`, `manifest-action`),
`cmd/run_inventory.go` (`observe`).

## Fix
Default `pudl list` now shows everything (no entry_type filter); `--artifacts`
narrows to run outputs (`manifest` + `manifest-action`). Because the artifacts
case needs two values, the single-value filter field became a list.

### API changes
- `database.FilterOptions.EntryType string` → `EntryTypes []string`
  (empty = no filter). WHERE clause now builds `entry_type IN (...)`.
- `lister.FilterOptions.EntryType string` → `EntryTypes []string` (passthrough).
- `cmd/list.go`: default `entryTypes = nil`; `--artifacts` →
  `["manifest","manifest-action"]`; `--all` → `nil`.
- Updated stale doc comment on `database.CatalogEntry.EntryType`
  (was `// "import" or "artifact"`).

### Callers updated
- `cmd/run_inventory.go` (`EntryTypes: []string{"observe"}`)
- `internal/mubridge/ingest_test.go`, `internal/mubridge/manifest_test.go`

## Verification
- Reproduced before fix (plain list + `--schema` → 0 rows).
- After fix against real catalog: plain list = 1178, `--schema pudl/linux.#Package`
  (no `--all`) = 599, `--artifacts` = manifest/manifest-action entries.
- `CGO_ENABLED=0 go test ./...` passes.

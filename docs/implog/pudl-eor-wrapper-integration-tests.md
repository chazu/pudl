# pudl-eor: Integration Tests for Collection Wrapper Import Flow

## Summary

Added three integration tests to `internal/importer/importer_test.go` verifying the full import flow with collection wrapper detection.

## Test Cases

1. **TestImportFile_CollectionWrapper** — Imports a JSON file containing `{"items": [{"id": "a"}, {"id": "b"}], "count": 2}`. Verifies:
   - Import returns a collection result with `RecordCount: 2`
   - Catalog has a collection entry with `CollectionType: "collection"`
   - Catalog has 2 item entries with `CollectionType: "item"` linked to the collection
   - Item data files exist on disk

2. **TestImportFile_NormalObjectNotDetectedAsWrapper** — Imports a standard JSON object. Verifies:
   - Single-object import path is unchanged (`RecordCount: 1`)
   - Schema is NOT `#Collection`
   - No collection items exist in the catalog for this entry

3. **TestImportFile_NDJSONUsesNDJSONPath** — Imports an NDJSON file (`.json` extension with multiple JSON lines). Verifies:
   - Format detected as `"ndjson"` (routed through NDJSON path, not wrapper path)
   - Schema assigned is `#Collection`
   - Individual item files created on disk

## Files Changed

- `internal/importer/importer_test.go` — Added ~75 lines (3 test functions + imports)

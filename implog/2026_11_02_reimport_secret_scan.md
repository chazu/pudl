# Re-import of a cataloged document is still scanned for secrets — pudl-zxq

`pudl import` deduplicates by content hash. For a single document the dedup
check ran before the document was decoded, so a re-import returned the
existing entry (`skipped`) without the likely-secret scan from `pudl-udi`.
Collections were not affected: their items are decoded and scanned while
staging (`collectionStream.prepare`), before the dedup check, and the tally
is applied to the skipped result too.

## Choice: scan before reporting the skip (option a)

The ticket allowed either scanning duplicates or noting in the output that
they were not rescanned. Scanning costs one extra decode of the staged bytes
of an already-cataloged document, which is bounded by the record byte limit
the import already enforces, and it reads only: the staged file is discarded
as before, so stored data, metadata and content hashes are unchanged. It
keeps one rule ("every imported record is scanned") instead of adding a
second output field and a pointer to `pudl doctor`. The CLI already prints
`importNotes` for skipped results (single and batch), and `--json` already
carries `secret_warnings`, so no output code changed.

## Change

- `internal/importer/enhanced_importer.go`: the dedup branch records that a
  document (not a collection) was found; after the catalog lock is released,
  `scanCatalogedDocument` runs. The scan sits outside the lock and does not
  touch `findByContentHashIn`.
- `internal/importer/secret_report.go`:

```go
// scanCatalogedDocument decodes the staged document and scans it under the
// intended (--schema) and the entry's stored schema, as importDocument does.
func (e *EnhancedImporter) scanCatalogedDocument(result *ImportResult, path, format, intended string) error
```

  A decode error fails the import with the same message as a fresh import;
  the bytes are identical to ones that decoded when first cataloged.

## Test

`TestReimportOfCatalogedDataStillWarns` (`internal/importer/secret_scan_test.go`)
imports a JSON document and an NDJSON collection, each with one secret, twice.
The second result is `Skipped`, has the same `secret_warnings` and content
hash as the first, and the stored bytes are unchanged. With the new call
disabled, the document case fails; the collection case passes either way.

## Line delta (`git diff --numstat` against `sre-focus`)

- Production Go: +25 / −1 (`enhanced_importer.go` +5, `secret_report.go`
  +20/−1).
- Tests: +25.

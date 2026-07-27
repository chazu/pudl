# Finish the bounded-memory importer (Recommendation 2)

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-bounded-importer.md`
**Closes:** Recommendation 2 from `docs/architecture-improvement-report.md`

## What was wrong

An earlier fix removed one whole-file read from the *hashing* step; everything
downstream still accumulated. `analyzeDataStreaming` read chunks off a channel
and appended every decoded object into one slice, so peak memory was the whole
decoded record set — for NDJSON and JSON arrays, unboundedly larger than the
file. Below 10 KB it took `analyzeDataDirect`, which read the file whole. Three
source passes (hash, analyze, copy) plus a full materialization.

The layering is what made this easy to miss: the accumulating call lived in the
embedded `Importer` while the pipeline above it read as though it streamed.

## What changed

**One staged pass, then one record at a time.**

- `stageSource` copies the source through an `io.MultiWriter` into a hasher and a
  temp file, then renames it into raw storage. One source read instead of two,
  and staging becomes atomic — a killed import leaves a temp file rather than a
  half-written record in the raw tree that a later read would treat as evidence.
- `streamNDJSON` and `streamJSONArray` decode incrementally into a `recordSink`
  that takes `json.RawMessage`. Raw bytes rather than a decoded value: an item's
  content hash comes from its canonical JSON anyway, so the sink avoids a
  decode/re-encode round trip and each record's residency is one buffer.
- A top-level JSON **array** is now detected by its first non-whitespace byte and
  routed to the element decoder. This is the case the report calls out and the
  one where the old path was worst: a 1 GB array of small objects became a 1 GB+
  slice before a single row was written.

**Identity is unchanged.** Still SHA256 of the raw source bytes, taken as read.
A test pins it against an independently computed hash.

**All-or-nothing is now a transaction.** The collection import runs inside
`WithCatalogTx` (Recommendation 4's first slice), so a failure rolls the rows
back instead of unwinding them through `cleanupFailedCollectionImport`'s
reconstructed paths. Because the record count is only known when the stream ends,
the order inverts: items stream first, the collection entry lands last with the
true count — which also removes the window in which a collection row existed
describing items that were never written. Item files are tracked and removed
after an abort; a rollback cannot unwrite a file, and an orphan file is wasted
disk where an orphan row is a lie.

**The legacy layer is retired.** `Importer` and `EnhancedImporter` are one type.
Nothing outside the package ever constructed the former — `NewEnhancedImporter*`
was always the only entry point.

**`GetLatestVersion` gained a transaction form**, so a stream that assigns
versions reads them inside the transaction that writes them. Without it two
records of the same resource in one import both read the same "latest" and both
claim the next version.

## Public API

- `internal/importer`: `Importer` and `NewWithSchemaPaths` removed (never
  reachable from outside). `NewEnhancedImporter*` unchanged.
- `internal/database`: `CatalogWriter` gained `GetLatestVersion`;
  `CatalogTx.GetLatestVersion`.
- Unexported: `stageSource`/`stagedSource`, `streamNDJSON`, `streamJSONArray`,
  `streamableCollectionFormat`, `recordSink`, `collectionStream`.

## Tests

`internal/importer/bounded_import_test.go` — NDJSON one record at a time (blank
lines are not records, a missing trailing newline is fine, an invalid line names
its line number, a sink error stops the stream); JSON arrays element by element,
empty, and non-arrays rejected; format routing including leading whitespace and
the single-object case that stays on the bounded path; staging preserves the
raw-byte hash, commits, and discards idempotently.

Two memory tests, which is what the report asks for ("measure peak memory rather
than only elapsed time"):

- `TestImportMemoryIsBoundedByRecordSize` compares `TotalAlloc` growth across a
  20× record count. A *ratio*, not an absolute, so it fails when someone
  reintroduces a slice of every record and not when the allocator shifts.
- `TestStreamingRetainsNothingAcrossRecords` states the property directly: at
  most one record is in the sink's hands at a time.

`internal/importer/collection_stream_test.go` — end to end through the real
importer: NDJSON and JSON arrays produce an item per record in stream order; the
collection's content hash is the raw file hash; a record repeated within one file
is one entry with two memberships (the dedup read goes through the transaction,
so it sees what this import just wrote); a mid-stream failure leaves no rows and
no item files; a duplicate import leaves no staged copy.

## Not done here

YAML and CSV still take the chunking parser. The report scopes this slice to
"NDJSON and large JSON arrays", and both remaining formats are single-document
in practice. The streaming package itself is not deleted — it still serves those
formats, and removing it is a separate decision.

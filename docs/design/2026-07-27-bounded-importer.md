# Design: finish the bounded-memory importer

**Date:** 2026-07-27
**Implements:** Recommendation 2 from `docs/architecture-improvement-report.md`
**Status:** stabilized after adversarial review (§6); ready to execute

## 1. Where the memory goes

An earlier fix removed one unconditional whole-file read from the *hashing* step.
Everything downstream still accumulates.

```
ImportFileWithFriendlyIDs
  → hash the file            (streamed — already bounded)
  → analyzeDataStreaming     → []interface{} of EVERY record, in memory
  → copyFile                 (streamed)
  → createCollectionItems…   → iterate that in-memory slice
```

`analyzeDataStreaming` reads chunks off a channel and appends every decoded
object into `allObjects`, so peak memory is the whole decoded record set — for
NDJSON and JSON arrays, unboundedly larger than the file. Below 10 KB it takes
`analyzeDataDirect`, which reads the file whole. Three passes over the source
(hash, analyze, copy) and one full materialization.

## 2. The staged stream

```
source
  ├─ stage:  one read → hash + write to a temp file → rename into raw storage
  └─ decode: one read of the staged file → one record at a time → record sink
                                                    ├→ schema/identity inference
                                                    └→ catalog entry + membership
```

One source read, one staged read, and **never more than one record resident**.

### 2.1 Staging computes the hash in the same pass

`stageSource` copies through an `io.MultiWriter` into a hasher and a temp file,
then renames. Identity is unchanged: the hash is still SHA256 of the raw source
bytes, and the test that pins it compares against the pre-change value.

The dedup check happens after staging, so a duplicate import writes a temp file
and removes it. That is the cheaper trade: the alternative is a second full read
of every *non*-duplicate import to hash before staging, and non-duplicates are
the common case.

Renaming rather than writing in place also makes staging atomic — a killed
import leaves a temp file, not a half-written record in the raw tree that a later
read would treat as evidence.

### 2.2 The decoder is incremental, per format

| Format | Decoder |
|---|---|
| NDJSON | `bufio.Reader` per line → one `json.RawMessage` |
| JSON array | `json.Decoder.Token()` to consume `[`, then `Decode` per element |
| JSON object | one `Decode` — a single record is bounded by definition |
| YAML, CSV | unchanged: not in this slice, and the report scopes it to "NDJSON and large JSON arrays" |

A JSON *array* is now detected and streamed rather than routed through the
chunking parser. This is the case the report calls out, and the one where the
current path is worst: a 1 GB array of small objects becomes a 1 GB+ slice.

### 2.3 The sink

```go
type recordSink func(index int, raw json.RawMessage) error
```

Raw bytes, not a decoded `any`: the item's content hash is computed from its
canonical JSON anyway, so handing the sink the bytes avoids a decode/re-encode
round trip and keeps the record's residency to one buffer.

### 2.4 All-or-nothing becomes a transaction

Today the collection entry is written first and a failing item triggers a manual
`cleanupFailedCollectionImport` that deletes rows and files by reconstructed
path. Recommendation 4 gave the catalog `WithCatalogTx`; the collection import
becomes one transaction, so a failure rolls the rows back rather than unwinding
them by hand.

The record count is only known when the stream ends, so the order inverts: items
stream first, the collection entry is written last with the true count. Inside one
transaction that is invisible to any other reader, and it removes the window in
which a collection row exists describing items that were never written.

Staged item files are tracked and removed on failure. A rollback cannot unwrite a
file, so the files are cleaned after the transaction aborts; an orphan file is
wasted disk, an orphan row is a lie.

### 2.5 Retiring the legacy layer

`EnhancedImporter` embeds `*Importer`, and `Importer` is never constructed
outside this package — `NewEnhancedImporter*` is the only entry point. The embed
is pure layering. The two types merge into one, `Importer` disappears, and the
streaming machinery it carried (`analyzeDataStreaming`, `analyzeDataDirect`)
survives only for the formats §2.2 does not stream.

## 3. Tests

| Case | Assertion |
|---|---|
| Content hash unchanged | SHA256 of raw bytes, byte-identical to the pre-change value |
| NDJSON import | every record becomes an item, in order |
| JSON array import | same, streamed rather than materialized |
| Single JSON object | unchanged behaviour |
| Item dedup | a repeated record reuses its entry and adds a membership |
| Failure mid-stream | no collection row, no item rows, no staged item files |
| **Peak memory** | importing N records allocates O(1) in N, not O(N) |
| Duplicate file import | skipped, and the temp staging file is removed |

The peak-memory test is what the report asks for ("measure peak memory rather
than only elapsed time"). It reads `runtime.MemStats.TotalAlloc` around imports
of 1× and 20× the record count and asserts the growth is sub-linear, with a
generous factor: the point is to catch a return to full materialization, not to
police allocation counts.

## 4. Non-goals

- YAML and CSV streaming. The report scopes this slice to NDJSON and JSON arrays.
- Removing the CDC/streaming parser package. It still serves the formats above,
  and deleting it is a separate decision.

## 5. Adversarial review

**A1 — "Staging before the dedup check writes a temp file for every duplicate
import."** *Accepted as the better trade.* The alternative costs a second full
read of every non-duplicate import, and non-duplicates are the common case. The
temp file is removed immediately on a dedup hit, and the test asserts it.

**A2 — "Two passes over the data (stage, then decode) is worse than one."**
*Rejected on what is actually being compared.* Today is three source passes
(hash, analyze, copy) plus a full materialization. This is one source pass, one
pass over the staged copy (warm in page cache, just written), and no
materialization.

**A3 — "Inverting the order so items precede the collection row changes
observable behaviour."** *Only inside a transaction, where nothing observes it.*
It also strictly improves the failure mode: the current order can leave a
collection row describing items that were never written, which is the window
`cleanupFailedCollectionImport` exists to paper over.

**A4 — "A peak-memory test will be flaky."** *Mitigated by measuring the right
thing.* `TotalAlloc` growth between two import sizes is a ratio, not an absolute,
and the assertion allows a generous factor. It fails when someone reintroduces
`append` into a slice of every record — which is the regression worth catching —
and not when the allocator's behaviour shifts by a few percent.

**A5 — "Merging `Importer` into `EnhancedImporter` is churn unrelated to
memory."** *Rejected: the report asks for it in the same sentence* ("finish the
bounded-memory importer rewrite **and retire the legacy importer layering**"),
and the layering is what made the memory-unbounded path easy to miss — the
accumulating call sits in the embedded type while the pipeline reads as if it
streams.

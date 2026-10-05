# Lossless numbers in collection import

**Date:** 2026-10-05

## Problem

The collection import path (NDJSON and top-level JSON arrays) decoded each
record with `json.Unmarshal` into `interface{}`, which turns every number into a
`float64`. The record was then re-marshalled to compute its content hash and
re-marshalled again (`json.MarshalIndent`) to write the stored item. An integer
above 2^53 was silently changed in the retained evidence and in its hash:
`{"id":9007199254740993}` was stored as `9007199254740992`, and two distinct
records could collapse into one item.

## Change

- `internal/idgen/canonical_json.go` (new) holds the number-preserving
  canonicalizer previously private to `internal/database/fact_identity.go`, so
  the importer can use it without importing the database package:
  - `DecodeJSONExact(raw []byte) (interface{}, error)` decodes one JSON value
    with `UseNumber`, rejects trailing content, and canonicalizes number
    spellings.
  - `CanonicalJSON(raw []byte) ([]byte, error)` returns sorted-key, canonical
    number JSON.
  - `NormalizeJSONNumbers(value interface{}) interface{}` and
    `CanonicalNumber(json.Number) json.Number` are the building blocks.
- `internal/database/fact_identity.go` and `query_values.go` now call
  `idgen.NormalizeJSONNumbers` / `idgen.CanonicalNumber`. Fact IDs are unchanged
  (same algorithm, same object-only fallback).
- `internal/importer/collection_stream.go` `writeItem`:
  - decodes records with `idgen.DecodeJSONExact`; inference and identity
    extraction receive `json.Number` values in canonical spelling;
  - hashes the canonical encoding of that value;
  - stores the record's own bytes, indented with `json.Indent`, instead of a
    re-encoding of the decoded value.

## Compatibility

`CanonicalNumber` uses encoding/json's float formatting cutoffs, so for any
record whose numbers survive a float64 round trip the canonical bytes, and
therefore the content hash, are identical to the old path. Re-importing data
imported before this change still deduplicates. Only records whose numbers did
not round-trip get a new (now correct) hash. Identity values are canonical
`json.Number`s, which marshal to the same text the old float64 values did, so
resource IDs are stable for the same records.

Behavior changes:

- Stored item files keep the source's key order, number spellings (`1.50`,
  `1e3`) and string escapes rather than sorted, re-encoded JSON. Their
  `size_bytes` changes accordingly for newly imported items.
- Numbers outside float64 range (e.g. `1e400`) no longer fail the record.

## Tests

- `internal/idgen/canonical_json_test.go`: parity with the float64 path for
  round-tripping values; digits kept where float64 loses them; trailing content
  rejected.
- `internal/importer/collection_numbers_test.go`: `9007199254740993` stored
  byte-exact and hashed distinctly from `…995`; stored bytes keep the record's
  spelling; `1.0`, `1e3`, `-0`, nested exponents and HTML characters keep the
  legacy content hash.
- Existing fact identity tests pass unchanged.

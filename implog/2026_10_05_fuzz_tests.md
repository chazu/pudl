# Native fuzz targets for parsers, canonicalizers, and the rule loader

**Date:** 2026-10-05

## Summary

Added seven Go native fuzz targets over the code that decides what pudl stores
and how it identifies it. Each has a seed corpus built from the shapes that
broke the former chunking parser (long repeated values, identical large list
items, CSV leading zeros and empty cells, integers beyond 2^53). Running them
found four real defects, fixed here with their crashers committed under
`testdata/fuzz`.

## Targets and properties

| Target | Package | Property |
|---|---|---|
| `FuzzDecodeJSONMatchesStdlib` | importer | `decodeDocument(json)` succeeds exactly when `json.Valid`, and yields the `encoding/json` + `UseNumber` value and record count |
| `FuzzDecodeYAMLMatchesYAMLv3` | importer | never panics; returns yaml.v3's non-empty documents in order (one value or a list) |
| `FuzzDecodeCSVVerbatim` | importer | one row per `encoding/csv` record after the header; cells verbatim; a duplicated header keeps its last occurrence the record reaches |
| `FuzzDetectFormat` | importer | content detection never errors; a valid JSON object/array within the 4KB sample is `json` or `ndjson`; `ndjson` is only reported for content that imports as NDJSON |
| `FuzzCanonicalJSON` | idgen | accepts exactly one valid JSON value; idempotent; whitespace-insensitive; numbers keep their exact value |
| `FuzzNormalizeIdempotent` | schemaname | never panics; `Normalize(Normalize(x)) == Normalize(x)` |
| `FuzzParseRulesCompile` | datalog | the loader never panics; every rule it accepts compiles, under current and as-of scopes, to SQL that SQLite prepares against a real catalog |

## Defects found and fixed

1. **Pretty-printed JSON detected as NDJSON** (`internal/importer/detection.go`).
   `isNewlineDelimitedJSON` counted lines that parsed and ignored the rest, so
   a single JSON document with two nested one-line objects was called NDJSON
   and the import then failed on its first fragment line. Every complete
   non-blank line in the sample must now be valid JSON; a line cut off by a
   full 4KB sample is ignored.
2. **`schemaname.Normalize` was not idempotent.** `"core.Item"` normalized to
   `"core.#Item"`, and normalizing that gave `"pudl/core.#Item"`: the
   legacy-prefix step ran before the `#` insertion. Repeated prefixes, version
   strips that exposed a prefix, and multiple colons had the same shape.
   Normalize now repeats its steps to a fixed point (bounded), and step 3
   rewrites every colon separator in one pass.
3. **`idgen.DecodeJSONExact` accepted trailing `]`/`}`.** `Decoder.More()`
   reports false before a stray closing bracket, so `0]` canonicalized as `0`.
   Current callers pre-validate, but the exported helper now requires a clean
   EOF after the value.
4. The CSV crasher (`a,a\n0`) was a wrong property, not a decoder defect: the
   oracle now mirrors the documented duplicate-header rule. The input is kept as
   a regression seed.

## Running

- `make fuzz` runs every target for `FUZZTIME` (default 30s).
- CI job `fuzz` runs `make fuzz` on pushes to `main` only, not pull requests.
- Seed corpora and committed crashers run as ordinary tests in `go test ./...`.

Each target ran 60s locally with no further failures.

## Public API

No new exported API. Behavior changes: `schemaname.Normalize` output for
inputs that previously needed two passes (`"core.Item"` now yields
`"pudl/core.#Item"`), and `idgen.DecodeJSONExact` rejects trailing closing
brackets.

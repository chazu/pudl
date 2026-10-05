# Import pipeline: stdlib decoders, honest --schema, import papercuts

**Date:** 2026-10-05

## Why

The import pipeline parsed every non-collection document (JSON and YAML ≥10KB,
all CSV) through a content-defined-chunking (FastCDC) parser that:

- dropped any chunk whose bytes it had already seen, so a 300,000-char repeated
  value was parsed as 168,928 chars, and a valid JSON document with ten
  identical ~68KB items parsed as **zero** objects (`Failed to import file`);
- split CSV rows at chunk boundaries and skipped the first row of every chunk
  (a 300-row file imported as 298 records), stripped leading zeros (`"00001"` →
  `1`) and turned empty cells into `"<nil>"`;
- printed a progress banner and warnings to stdout on every import;
- tripped Go's checkptr instrumentation (pudl-1ih), which CI masked with
  `-gcflags=all=-d=checkptr=0`.

It also accumulated every object in memory, so the chunking bought nothing.
Separately, `--schema` was ignored for single documents and assigned to
collection items without validation, and compressed files were only
decompressed for format detection (a `.json.gz` Deployment imported as 123
compressed bytes classified as the catch-all).

## What changed

### Decoding (`internal/importer/decode.go`)
- New `decodeDocument(r, format)` built on `encoding/json` (with `UseNumber`, so
  integers beyond 2^53 survive), `yaml.v3` (multi-document aware),
  `encoding/csv` (cells verbatim), and an NDJSON reader. Unstructured input gets
  a descriptive object (`format`, `content`, line/char/byte counts; binary input
  gets `encoding: binary`).
- Exported `DecodeFile(path, format)` for callers outside the pipeline.
- `analyzeDataDirect`/`analyzeDataStreaming` replaced by `analyzeData`.
- Deleted `internal/streaming/` (~3,000 lines), `examples/streaming_demo.go`,
  and the `github.com/PlakarKorp/go-cdc-chunkers` dependency. CI's race job no
  longer disables checkptr. Resolves pudl-1ih.

### Compression
- `DecompressTo(sourcePath, dir)` decompresses into a named temp file that keeps
  the inner extension; `DecompressFile` delegates to it.
- `ImportFileWithFriendlyIDs` decompresses once, up front, into the workspace's
  `data/tmp`. Hashing, raw storage, size, and decoding all use the decompressed
  bytes. **Deliberate:** the content ID of a compressed import is now the hash
  of its decompressed content, so a re-import of a `.gz` imported by an older
  pudl will not dedup against that earlier entry.

### Schema assignment (`internal/importer/schema_assign.go`)
- `assignSchema` is shared by single documents and collection items. Without
  `--schema` it infers. With one, it runs `ChainValidator.ValidateChain`
  (intended → base → catchall) and records the `ValidationResult`; data is never
  rejected. A manual schema the validator has not loaded (e.g. a
  module-versioned ref such as `mu/aws@v1#EC2Instance`) is recorded as given at
  confidence 0.5.
- The importer builds a chain validator on demand when the caller supplies none.
- `.meta` `validation_status` is `auto-assigned`, `validated`, or `fallback`.
- `internal/validator/chain_validator.go` (outside this package, a one-function
  change): validation now uses the same rule as inference —
  `Validate(cue.Concrete(true))`, structural only for list schemas. Previously a
  record missing a required field "satisfied" any schema it did not conflict
  with.

### Single-document path (`internal/importer/document.go`)
- The catalog/metadata half of a single-document import moved to
  `importDocument`. A failure after the raw file is committed now removes the
  raw file and metadata instead of leaving evidence with no catalog row.
- Directories are rejected with a clear error instead of failing in `io.Copy`.

### CLI (`cmd/import*.go`)
- `cmd/import.go` (728 lines) split into `import.go`, `import_session.go`,
  `import_batch.go`, `import_stdin.go`, `import_envelope.go`.
- `importSession` gives single, batch, and stdin imports one importer and one
  `--schema` resolution (`ResolveSchemaName`), which batch and stdin previously
  skipped.
- `--path` is no longer cobra-required, so `cat x | pudl import` reaches the
  stdin path; with no `--path` and a TTY on stdin, the missing-path error is
  unchanged.
- Stdin is staged under a unique temp name (`pudl-stdin-*.tmp.<ext>`) rather
  than the shared `stdin.<ext>`.
- Envelope payloads are staged under the workspace `data/tmp`, not beside the
  source (read-only source directories now work), and origin is derived from the
  user's file, not the `.envelope-*` temp name.
- `recordItemSchemas` writes through the importer's open catalog instead of
  opening a new `CatalogDB` (migrations, view rebuild) per file.
- `--streaming-memory` / `--streaming-chunk-size` are accepted, hidden, and
  deprecated; they have no effect.
- The validation display lists the intended schema's issues inline; the old
  hint pointed at a nonexistent `pudl show --validation` flag.

### Other fixes
- JSON-array collections are recorded with format `json` (was `ndjson`), and
  the collection entry's schema is normalized.
- `pudl schema reinfer` decodes stored entries with `DecodeFile` using the
  entry's format (YAML/CSV/NDJSON were reinferred from raw text, i.e. as the
  catch-all) and decompresses entries stored compressed by older imports.

## Behavior changes

- CSV records are maps of verbatim cell text, always a list (a one-row CSV used
  to yield a single map with `_column_count`/`_row_number` keys).
- Parsed JSON numbers are `json.Number`. Identity values written from non-
  canonical number literals (e.g. `1.0`) canonicalize differently than before.
- Compressed imports hash and store decompressed bytes (see above).
- `--schema` now actually assigns or falls back based on validation.

## Public API

Package `internal/importer`:
- `func DecodeFile(path, format string) (interface{}, int, error)`
- `func DecompressTo(sourcePath, dir string) (string, error)`
- `func StdinExtension(format string) string` (replaces `GetStdinFilename`)
- `func (e *EnhancedImporter) CatalogDB() *database.CatalogDB`
- `func (e *EnhancedImporter) TempDir() (string, error)`
- `ImportOptions.OriginPath string`; removed `UseStreaming` and `StreamingConfig`

Package `internal/streaming`: removed.

## Tests

- `internal/importer/decode_test.go`: repeated 300K-char value preserved;
  ten identical ~68KB items parse; 300-row CSV keeps `"00001"` and `""`;
  large integers keep precision; trailing content rejected; multi-document
  and large YAML.
- `internal/importer/import_regression_test.go`: full-size import of a
  repetitive document; `.json.gz` decompressed through the pipeline
  (Deployment classified as `pudl/k8s.#Resource`, plain re-import dedups, temp
  removed); `--schema` honored and validated on documents and collection items;
  JSON-array collection format; `OriginPath`; directory rejection.
- `cmd/import_stdin_test.go`: `--path` not required; piped stdin import;
  envelope from a read-only directory keeps the original origin.

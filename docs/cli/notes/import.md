Behavior:

- Duplicate files (same content hash) are skipped automatically.
- Without `--path`, data is read from piped stdin; `--format` names its format.
- A directory imports its supported files; `--recursive` descends into
  subdirectories. Hidden entries are skipped.
- NDJSON files and top-level JSON arrays are split into collections with
  individual items. Numbers are kept exactly as written.
- CSV rows keep every cell verbatim as text (`"00123"` stays `"00123"`).
- `.gz` and `.zst` files are decompressed; the stored data, hash, size and
  inference all use the decompressed content.
- Typed envelope JSON (`schema`, optional `definitions`, and `data`) is
  unwrapped; its schema metadata is recorded and the inner payload follows
  normal import.
- `--schema` validates the data against that schema and falls back down its
  `base_schema` chain when the data does not satisfy it; data is never rejected.
- Format is detected from extension and content; origin from the filename.

Set `PUDL_DEBUG=1` for detailed error output.

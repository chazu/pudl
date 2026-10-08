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

`--explain` exposes classification/fallback reasons and schema sources. Byte limits bound records, decoded input, and staging; failures never truncate records. See [evidence](evidence.md).

Paths can be given as arguments too (`pudl import a.json exports/*.json`), so a
shell-expanded glob imports every file. Named paths win over piped stdin.

`--set path=value` (repeatable) writes a field into every record before
anything else happens — e.g. the `project` gcloud leaves out. It overwrites;
values are typed like JSON (`'"123"'` forces a string). JSON/NDJSON only.

`--dry-run` classifies, identifies, redacts and projects every record and
writes nothing: per-schema counts, records already cataloged (and how many a
`--schema` would move), validation failures with their first issues,
unresolved identity, projected facts and records that matched more than one
schema family.

Re-importing data that is already cataloged with a `--schema` it satisfies
moves those entries to that schema. A re-import is also an observation: the
records' projected facts become current again.

Records of schemas declaring `sensitive_fields` are redacted before storage;
schemas declaring `facts` project into the fact store. See
[projection](projection.md).

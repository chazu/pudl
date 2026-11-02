# Doctor accepts JSON array collections; facts search escapes punctuated terms

Addresses `pudl-f7z` (P1) and `pudl-jfs` (P3), both found during the
2026-11-02 docs rewrite.

## pudl-f7z: doctor reported every JSON array import invalid

**Bug.** `pudl import fw.json` with a JSON array stores a collection entry
assigned `pudl/core.#Collection` plus one entry per item. `pudl doctor`
reported the collection `invalid — All validations failed`, so doctor and every
`doctor --fix` (including `--dry-run` and `--only NAME`) exited 1, and printed
the command usage after the error.

**Root cause.** Doctor validated each entry's stored payload against its
schema. A collection's stored payload is the original array (or NDJSON stream),
but the built-in `#Collection` describes collection metadata (`collection_id`,
`item_count`, ...), so it could never match. Inference was already skipped for
collections; validation was not. Fan-out `#CommandObserve` ingests create the
same kind of collection entry.

**Fix.**

- `internal/doctor/catalog_check.go`: `isCollectionContainer(entry)` is true
  for a collection entry assigned the built-in `#Collection`; doctor does not
  validate its payload (the items are validated as their own entries). A
  collection assigned a list-type schema is still validated. The secret scan
  still covers the payload.
- `cmd/doctor.go`: `SilenceUsage: true`, so a failed diagnosis prints the
  error, not usage.

Doctor's exit status still reflects catalog validation, so a real invalid entry
still fails `doctor --fix`; that is unchanged.

## pudl-jfs: facts search errored on hyphenated terms

**Bug.** `pudl facts search prod-nat` failed with `SQL logic error: no such
column: nat`: the query went to FTS5 `MATCH` unescaped, and FTS5 parses `-` as
an operator.

**Fix.** New `internal/database/facts_fts_query.go`: `ftsQuery` rewrites the
query before `MATCH`. Bare word terms, `"quoted phrases"`, `AND`/`OR`/`NOT`,
parentheses and a trailing `*` pass through unchanged, so the documented FTS5
syntax still works. Any other term (`prod-nat`, `10.0.0.1`) is quoted as a
phrase, keeping a trailing `*` as a prefix match (`"prod-nat"*`); embedded
quotes are doubled. `SearchCurrentFacts` uses it.

Help text, `docs/facts.md` and `docs/cli/notes/facts_search.md` now say
punctuated terms match as phrases (they previously told users to quote them);
the help golden and `docs/cli-reference.md` are regenerated.

## Public API

No exported API changes. New unexported `database.ftsQuery`,
`doctor.isCollectionContainer`.

Line delta: production Go +80/−3 (doctor +13, FTS escaping +68).

## Tests

- `cmd/doctor_collection_test.go`: a JSON array import leaves doctor `ok`
  for the collection and both items, `doctor --fix --dry-run` succeeds, and a
  failing doctor prints no usage. Failed before the fix.
- `internal/database/facts_fts_query_test.go`: `ftsQuery` cases, and searches
  for `prod-nat`, `10.0.0.1` and `prod-n*` against stored facts. The search
  test failed before the fix with the reported error.

## Validation

See the merge gate in `implog/2026_11_02_bugfix_round.md`.

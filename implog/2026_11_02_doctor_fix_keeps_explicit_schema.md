# `doctor --fix` keeps explicit `--schema` assignments (pudl-ww1)

A plain `pudl doctor --fix` reclassified entries imported with an explicit
`--schema` whenever inference preferred another schema. The `inference`
repair is documented to cover only inferred entries unless
`--include-explicit` is given.

## Root cause

Two gaps:

1. Detection ignored the record. Doctor's catalog scan
   (`shouldVerifyInference`) treated an entry as explicit only when it had an
   `entry_type`, a declared `_schema` or a collection type. A plain
   `pudl import --schema X` has none of these. Its explicitness lives only in
   its `.meta`, which nothing read. The scan reported the entry as a
   `mismatch`, so doctor failed. It also put the entry on the drift list that
   the `inference` repair applies.
2. The record was incomplete. `schema_info.intended_schema` was never written.
   `validation_status` was `auto-assigned` for an explicit schema kept
   unverified (unavailable with `--allow-schema-fallback`) and for every
   collection item. A re-import with `--schema` that moved an existing entry
   did not touch its `.meta` at all. After such a move, `doctor --fix` moved
   the entry back.

`#CommandObserve` ingest entries carry `entry_type: observe` and were already
skipped. A policy-leaves move does not change the `.meta`, so an explicit
entry stays explicit after one.

## Fix

- `importer.ExplicitAssignment(entry, data)` is now the one test for
  explicitness. An entry is explicit when it has any `entry_type`, its record
  declares `_schema`, or its `.meta` records a requested schema. A requested
  schema is either an `intended_schema` or a `validation_status` of
  `validated|fallback|unverified`; the status covers imports made before this
  fix.
- Doctor's scan uses it (`cmd/doctor_catalog.go`, a one-line change). The
  `inference` repair also checks it for every entry it is handed, unless
  `IncludeExplicit` is set. The repair therefore keeps its contract whatever
  list of drifted entries the caller passes in.
- Every explicit assignment now writes `intended_schema` to the `.meta`. This
  covers documents, collection items, and the transform pre-pass spool.
  `validation_status` comes from that intent: `auto-assigned`, `validated`,
  `fallback`, or `unverified`. Collection items no longer hard-code
  `auto-assigned`.
- When a re-import moves an existing entry to an explicit schema, the entry's
  `.meta` `schema_info` is rewritten to the new assignment. This happens after
  the catalog transaction commits, and the other sections are kept.

## Public API

- `func importer.ExplicitAssignment(entry database.CatalogEntry, data any) bool`

## Tests

- `cmd/doctor_repairs_test.go`
  - `TestDoctorFixKeepsExplicitImportSchema`: an import with `--schema`
    survives diagnosis and a plain `--fix`. `--include-explicit --entry`
    reclassifies the entry. This test failed before the fix: the repair status
    was `needed`.
  - `TestDoctorFixKeepsExplicitReassignment`: an inferred import is moved by
    a re-import with `--schema`, and the move survives `--fix`. This test
    fails without the `.meta` rewrite.
- `internal/repair/inference_test.go`: the repair skips explicit entries
  even when the drift list names them, including a legacy `.meta` with only
  `validation_status: validated`.
- `cmd/doctor_scrub_test.go` now runs a real `doctor --fix` instead of
  `--dry-run`, and asserts that the inference repair status is `ok`.

## Line delta (against sre-focus 19da787)

| Code | Added | Removed | Net |
| --- | --- | --- | --- |
| Production Go | +156 | -22 | +134 |
| Test Go | +123 | -3 | +120 |

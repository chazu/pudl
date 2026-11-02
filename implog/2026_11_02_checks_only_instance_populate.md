# Checks-only model instances validate in `pudl doctor` (pudl-qhx)

## Problem

A checks-only model (only `name` and `checks`) was recorded in the catalog
with `populate: {}`. `#CommandObserve` requires `runs`, so the instance failed
`#SystemModel` validation, and `pudl doctor --json` showed the entry `invalid`
with `ok: false`.

## Root cause

`SystemModel.Populate` is a value struct tagged `json:"populate"`. The checks-only
marker `Absent` is `json:"-"` and every other field is `omitempty`, so
`recordModelInstance` (`cmd/run_resolve.go`) serialized an empty object. Without a
tag change, `omitempty` cannot drop a struct.

## Fix

- `internal/systemmodel/systemmodel.go`: the field is tagged
  `json:"populate,omitzero"`. `Populate.IsZero()` reports true when the arm is
  `Absent` or carries no field. A checks-only instance now has no `populate` key.
  A command model serializes as before. The fix is in the serialization layer,
  so it holds wherever the code that stores the instance moves.

## Legacy data

Older catalogs keep the invalid instances. Re-running `pudl check` adds a valid
instance but does not remove the old content-addressed entry, so doctor would
still fail.

Doctor now tolerates that exact shape. `systemmodel.WithoutLegacyEmptyPopulate`
returns a `#SystemModel` payload whose `populate` is an empty object with the
key removed. In `cmd/doctor_catalog.go`, `validateCatalogData` retries
validation on that copy only when the stored payload fails.

A repair that rewrites the payload was rejected. Rewriting a payload changes the
entry's content-addressed ID, which needs the ID re-derivation, redirects and
compaction of the sensitive scrub. That cost buys no information, because
`populate: {}` already meant "no populate arm". The tolerance is read-only and
matches that one shape only. Any other invalid instance is still reported.

## Public API

- `systemmodel.SchemaName`: `"pudl/systemmodel.#SystemModel"`.
- `systemmodel.WithoutLegacyEmptyPopulate(schema string, data any) (any, bool)`.
- `systemmodel.Populate.IsZero() bool`.

## Tests

- `cmd/checks_only_instance_test.go`: `pudl check` on a checks-only model,
  then `pudl doctor --json`. The instance is `ok` and `ok: true`; this failed
  before the fix. A legacy `populate: {}` instance ingested beside the new one
  is also `ok`.
- `internal/systemmodel/legacy_test.go`: marshal omits `populate` for
  checks-only and zero models and keeps it for command models. The legacy
  helper matches only the exact shape and does not mutate its input.

## Line delta

Production Go: +58 / -2. Tests: +114. Docs: +64.

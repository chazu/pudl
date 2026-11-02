# schema add / schema new: drop legacy metadata (pudl-ltq)

## Bug

- `pudl schema add` ran `internal/schema/validator.go`, which grepped the file
  for `_identity:`, `_tracked:` and `_version:` and warned "Missing _identity
  field in schema definitions" (and the same for the other two) on every
  schema. Those fields are not pudl metadata; pudl reads `_pudl`
  (`identity_fields`, `tracked_fields`, `review`, ...). Every correct schema
  got three bogus warnings.
- `pudl schema new` (`internal/schemagen/generator.go`) wrote
  `cascade_fallback: ["pudl/core.#Item"]` into the generated `_pudl` block. The
  key went away with the cascade priority system and nothing reads it.

## Fix

Deleted the dead checks; no new machinery.

- `internal/schema/validator.go`: removed `MetadataInfo`, `validateMetadata`,
  the `HasMetadata` / `MetadataInfo` fields of `ValidationResult` and the three
  warnings. `ValidationResult` is now `Valid`, `Errors`, `Warnings`,
  `PackageName`, `Definitions`. The remaining warnings (no package
  declaration, no `#Definition`) are unchanged.
- `internal/schemagen/generator.go`: no longer writes `cascade_fallback`.

Why not check `_pudl` in `schema add`: the load path already decodes `_pudl`
strictly (`internal/validator/pudl_strict.go`, `review.go`, `owner.go`,
`secret_scan.go`) and `CheckModuleIntegrity` reports a bad `review`,
`secret_scan_ignore` or `owner` as a schema load error, surfaced by
`LoadSchemaSet` on the next load (`pudl doctor`, `schema list`, import).
`schema add` validates a standalone file outside the workspace module, so
reusing that decode would mean building a module load for it, which is not a
cheap call. Dropping the warnings avoids a second, divergent `_pudl` check.

No CLI help text changed. No current docs, skills or testdata described the
removed warnings or `cascade_fallback`; historical records (implog/,
`docs/vestige-sweep.md`) were left alone.

Public API change: `schema.ValidationResult` loses `HasMetadata` and
`MetadataInfo`; `schema.MetadataInfo` is removed. Only `cmd/schema_add.go`
uses the validator and it read neither.

## Net production line delta

-43 lines (`validator.go` -42, `generator.go` -1).

## Tests

- New `internal/schema/validator_test.go`:
  `TestValidateSchemaNoLegacyMetadataWarnings` — a schema with a `_pudl` block
  validates with no warnings (failed before the fix with the three legacy
  warnings).
- `internal/schemagen/review_test.go`: `TestGeneratedSchemaStartsAsDraft` now
  also asserts the generated content has no `cascade_fallback` (failed before
  the fix).

## Validation

- `CGO_ENABLED=0 mise exec -- go test -count=1 ./...` — all packages pass.
- `CGO_ENABLED=0 mise exec -- make lint` — 0 issues.
- `make check-docs check-skills` — pass.

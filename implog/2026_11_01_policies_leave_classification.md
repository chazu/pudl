# Policies leave classification — pudl-xf5.8.1

Delivery step 1 of `docs/design/2026-11-01-compliance-policies.md` ("Decision"
and "Classification changes"). A policy schema (`_pudl.schema_type:
"policy"`) is evaluated against resources and never assigned to one. This
step removes policies from inference, from validation fallback, and from
explicit schemas, and adds a `pudl doctor --fix` repair for entries that were
classified to a policy before.

## Public API

```go
// internal/validator (policy_schema.go)
const SchemaTypePolicy = "policy"
func (m SchemaMetadata) IsPolicy() bool
// NearestType walks base_schema to the first non-policy schema; "" if none.
func NearestType(schema string, lookup func(string) (SchemaMetadata, bool)) string
type PolicySchemaError struct{ Policy, Type string } // Error() string
func (cv *ChainValidator) CheckAssignable(schema string) error // *PolicySchemaError for a policy

// internal/policy: SchemaTypePolicy now aliases validator.SchemaTypePolicy.

// internal/repair: new registered repair "policy-leaves", run after
// schema-refs and before inference.
```

## Behavior

- **Inference.** `scoreCandidate` scores a policy 0, so `SelectCandidates`
  never returns one, and the all-schemas fallback (used when no heuristic
  scores) skips policies. Type-to-type inheritance is unchanged: a custom
  subtype is still preferred over its base.
- **Validation chain.** `ValidateChain` returns a `*PolicySchemaError` when the
  intended schema is a policy. `buildValidationChain` walks through a policy
  ancestor without adding it as a fallback.
- **Explicit schemas.** `validateExplicitSchema` (import and `--dry-run`
  preview) rejects a policy before reading the source, even with
  `--allow-schema-fallback`. The error names the policy's nearest type and
  points at `pudl policy evaluate` as the follow-up:
  `X is a policy schema, not a type: name its type T instead; compliance with X
  is evaluated separately (`pudl policy evaluate`, a follow-up)`.
- **Command arm.** A `#CommandObserve` `populate.schema` does not go through
  importer options: `cmd/run_populate.go` builds a `ChainValidator` and
  `mubridge` validates each record with `ValidateChain`, which now returns the
  same error. No `cmd/run*` or `internal/mubridge` file changed (both are being
  removed or reworked by `pudl-z87`).
- **Repair `policy-leaves`.** Diagnosis lists catalog entries whose assigned
  schema is a loaded policy, with the type each would move to. `--fix`
  updates only the schema column; `resource_id`, `identity_json`, version and
  facts are kept, because identity is namespaced by the family root that a
  policy shares with its type. `--dry-run` writes nothing. A policy whose chain
  reaches no type is reported as a failure. Doctor's projection sync runs after
  any change, as for the other repairs.

## Docs

- `docs/schema-authoring.md`: the `schema_type` table, `base_schema`, the
  two-phase classification description, the reclassification section (adds
  `--only policy-leaves`) and two tips no longer say policies are classified.
  The policy section itself is rewritten by `pudl-xf5.8.9`.
- `pudl import` help no longer lists policies in the validation chain and its
  `--schema` example no longer names a compliance schema; `pudl doctor` help
  lists `policy-leaves`. `docs/cli-reference.md` and the help golden file are
  regenerated; the embedded `pudl-core` skill's repair list is updated.

## Tests

- `internal/inference/policy_test.go`: inference never returns or tries a
  policy, through heuristic candidates and the all-schemas fallback; a custom
  subtype still wins. Fails on the previous code (every case picked the policy).
- `internal/validator/policy_schema_test.go`: `ValidateChain` rejects a policy
  with its type named; subtypes stay assignable and fall back to their base;
  the fallback chain skips a policy ancestor; `NearestType`.
- `internal/importer/strict_schema_test.go`: `--schema` naming a policy fails in
  preview and import, with and without fallback, catalogs nothing, and an
  inferred import of the same file assigns the type.
- `internal/repair/policy_leaves_test.go`: a catalog fixture with one resource
  observed as a type and as two policy leaves; diagnosis and dry run find two
  entries without writing; the fix moves both to the type with resource_id,
  identity_json and versions unchanged; the kept resource_id equals the one an
  import under the type derives; a second diagnosis is clean.

## Line delta against `sre-focus` (323f63c)

- Production Go: +162 / -11, net +151 (two new files of 64 lines each).
- Tests: +383, net +383.
- Docs, help golden file and skills: +28 / -16 (not counting this log and
  `docs/plan.md`).

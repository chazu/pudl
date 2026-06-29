# 2026-06-29 — Rename the "cascade" validator to "chain" (vestige sweep §3.2)

## What & why

The validation subsystem still carried the name of a **removed** feature. The old
"cascade priority" validator (schemas with `cascade_priority` / `cascade_fallback` /
`compliance_level`, walked in priority order) was replaced by native CUE unification
in `f79297b`. The live code now just builds a chain `intended → base_schema chain →
catchall (pudl/core.#Item)` and tries `schema.Unify(data).Validate()` at each level,
first success wins — no priority numbers, no compliance levels. Only the **name** was
a fossil. This is a pure rename (no behavior change), closing tracker item §3.2.

## Rename map (validator subsystem only)

| Before | After |
|--------|-------|
| `internal/validator/cascade_validator.go` | `internal/validator/chain_validator.go` |
| `CascadeValidator` | `ChainValidator` |
| `NewCascadeValidator` | `NewChainValidator` |
| `ValidateWithCascade` | `ValidateChain` |
| `CascadeAttempt` / `CascadeAttempts` | `ChainAttempt` / `ChainAttempts` |
| `AddCascadeAttempt` | `AddChainAttempt` |
| `ServiceCascadeAttempt` | `ServiceChainAttempt` |
| `convertCascadeAttempts` | `convertChainAttempts` |
| json `cascade_attempts` | json `chain_attempts` |
| comments/help "cascading/cascade validation" | "chained/chain validation" |

Callers updated: `internal/importer/importer.go` (the `ChainValidator` opts field +
`ValidateChain` call), `cmd/import.go` (local var + help text), and
`internal/importer/importer_test.go`.

The receiver `cv` and the validation flow are unchanged.

## Verified safe before taking the rename all the way

`ServiceValidationResult` (with the `chain_attempts` json tag) is **never marshaled
to output** — `cmd/validate.go` only uses `json.Unmarshal` on the *input* data and
renders results via `GetValidationSummary` (human text). So renaming the json tag has
no external contract impact.

## Explicitly left alone (distinct concerns, not the fossil)

- `--cascade` collection delete: `cmd/delete.go`, `lister.DeleteEntry(... cascade bool)`,
  `TestCollectionCascadeOperations` — a legitimate cascade-delete.
- `internal/inference`: `CascadePath`, `GetCascadeChain`, the "cascade priority" comment
  in `heuristics.go`, `cascade_fallback` emitted by `schemagen/generator.go` — inference
  ordering / generated-schema scaffolding, a separate question from §3.2.

## Result

`CGO_ENABLED=0 go build ./...` clean; `CGO_ENABLED=0 go test ./...` all green.
Tracker `docs/vestige-sweep.md` §3.2 marked DONE.

## Public API delta

None functional. Exported type `validator.CascadeValidator` and its methods are now
`validator.ChainValidator` / `ValidateChain`; `validator.ServiceValidationResult.CascadeAttempts`
is now `ChainAttempts`. All callers in-tree updated.

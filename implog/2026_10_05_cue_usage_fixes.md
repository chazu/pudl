# CUE usage fixes

Date: October 5, 2026.

Three defects found while assessing CUE v0.18.0-alpha.2 (first-class
functions) for pudl. The assessment concluded functions are not worth adopting
yet: they require language version v0.18.0, whose now-stable strict embedding
breaks `internal/systemmodel/schema.cue`, and function values are opaque to the
Go API (no field iteration, decode, or call). Identity extraction is the one
plausible future fit.

## `pudl rule new` scaffold could not be loaded

The scaffold wrote a `#Name: r.#Rule & {...}` definition importing
`pudl.schemas/pudl/rules@v0`. The rule loader compiles each file standalone
(no imports) and skips definitions, so the file failed to compile and every
`pudl query` errored until it was deleted. The scaffold now emits a plain
top-level field with a quoted label, matching the shipped rules and
`pudl guide`.

- `cmd/rule_new.go`: `ruleScaffoldSource(name string) string`.
- Test: `cmd/rule_new_test.go`.

## Cross-context unification in the validation chain

`ChainValidator.ValidateChain` compiled record JSON in a private
`cue.Context` and unified it with schemas from the per-path `SharedLoader`
contexts. CUE permits values from only one Context per operation; it worked
only because label interning is currently global. Data is now compiled once
per schema context, and the unused private context field is removed.

- `internal/validator/chain_validator.go`.
- Test: `internal/validator/chain_validator_test.go` (chain spanning two
  schema paths).

## Inference recompiled data per candidate

`SchemaInferrer.Infer` recompiled the record JSON for every candidate schema.
It now compiles once per CUE context per call (`tryUnify` takes a
`map[*cue.Context]cue.Value` memo). With 28 candidates and no match against
the bootstrap schemas: ~1.09ms → ~0.58ms per record, allocations 11.1k → 6.1k.

- `internal/inference/inference.go`.

No public API changed.

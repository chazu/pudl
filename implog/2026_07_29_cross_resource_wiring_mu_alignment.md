# Cross-resource wiring design and mu secret alignment

**Date:** 2026-07-29
**Design:** `docs/design/2026-07-28-cross-resource-value-wiring.md`

## Outcome

The cross-resource value-wiring design completed adversarial review and was
checked against mu's current secret input/output contracts. The deferred secret
phase now has explicit extension points instead of an unspecified PUDL-only
secret mechanism.

## Design decisions

- Scalar cross-resource values are elaborated through CUE before the model is
  decoded and rendered to mu.
- Future secret bindings must lower to mu `sealed_inputs` and
  `sealed_input_modes`, not concrete CUE values, catalog records, or reports.
- Execute pith must use `secret/get`, preserving mu/pith taint and redaction
  behavior; `env/get` remains the non-secret path.
- Producer-side secret capture must use `sealed_outputs` and the provider
  `store_secret` capability, with mu's existing `sealed_output_modes` and
  `secrets.writable_refs` policy.
- The provider boundary remains mu's `resolve_secret`/`store_secret` protocol,
  Go SDK handlers, and `SecretBackend`/`SecretPlugin` adapter.
- File delivery remains subject to mu's current `0600` temporary-file behavior
  and toolchain-sandbox limitation.
- Observation reuse follows a convention-over-configuration rule: current-run
  producer snapshots take precedence, standalone consumers use the latest
  successful scoped snapshot, and stricter age bounds stay at run policy level
  rather than in every CUE binding.

## Public API

No runtime or public API code changed. This slice records the compatibility
contract that the future implementation must reuse.

## Evidence

- Cross-checked the design against mu's `internal/config/schema.cue`, sealed
  input delivery documentation, secret write policy, plugin protocol, SDK
  guide, pith plugin guide, and pith sealed-I/O design.
- `git diff --check` passes for tracked changes; the new design and implog files
  are documentation additions with no trailing whitespace beyond intentional
  Markdown hard breaks.

# Typed projection and validation alignment

**Date:** 2026-07-28
**Design:** `docs/design/2026-07-27-swamp-parity-roadmap.md`

## Outcome

Closed the typed projection/validation gap in the shipped PUDL path. Catalog
assignments now have one canonical schema namespace, explicit plugin and bridge
records are validated against their stored assignment, and inference fixed-point
checks apply only to ordinary untyped imports.

## Implementation

- The CUE loader derives schema names from the schema-root-relative directory,
  preserving paths such as `pudl/k8s.#Resource` instead of collapsing them to
  the CUE package name `k8s`.
- Component-only CUE packages remain loadable without becoming phantom catalog
  schemas; tracked definitions still undergo module-integrity validation.
- Explicit plugin mappings normalize legacy CUE references before checking the
  loaded inheritance graph, so only loaded canonical schemas are persisted.
- `pudl validate` reports a missing assigned schema directly instead of hiding
  the problem behind generic fallback output.
- `pudl verify` validates every stored assignment first. It re-runs inference
  only for untyped imported records; bridge entries, collections, and records
  carrying an explicit `_schema` are treated as producer-assigned.
- Shipped schemas now cover the persisted bridge payloads (`#DriftObservation`,
  `#ManifestAction`, snapshot `run_id`), SystemModel routing metadata, and the
  nullable optional fields emitted by the AWS observer.

## Evidence

- Focused validator, mubridge, and command tests pass.
- `mise exec -- go test ./...` passes.
- `mise exec -- go vet ./...` passes.
- `mise exec -- go build ./...` passes.
- `go run ./internal/skills/gen -check`, `git diff --check`, and `br lint` pass.
- A deterministic mixed catalog fixture with 41 entries passes `pudl validate
  --all` with 41 valid / 0 invalid and `pudl verify` with 41 OK / 0 mismatch.

## Boundary

Cross-model value wiring remains deferred. The next slice must begin with one
concrete producer/consumer value-flow request and define freshness, failure,
absence, and unresolved-value behavior before adding interpolation or lazy
catalog functions.

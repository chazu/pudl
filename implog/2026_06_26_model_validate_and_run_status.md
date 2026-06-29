# `pudl model validate` + run-loop status persistence (Cluster A step 2)

**Date:** 2026-06-26

## Summary

Second step of the Cluster A plan (docs/vestige-sweep.md §6): the salvaged-value
pieces of World A re-homed on the SystemModel spine — pre-run validation and
per-definition status — plus the build-spec §5 status-honesty fix.

## Public API

- **`pudl model validate <name>`** — resolve + structurally validate a
  `#SystemModel` without running it. Beyond the CUE decode that `resolveModel`
  already does, it flags: an observe arm with no plugin name; an ewe arm missing
  `eweSource` or `outputs`; a convergent model with empty `desired`; and any
  `desired` entry missing its quoted `"_schema"` routing tag. Exits non-zero on
  problems. (`cmd/model_validate.go`)
- **`pudl model list`** now shows a **STATUS** column (and `--json` a `status`
  field) — the model's last-run verdict, best-effort from the catalog.

## Run-loop status persistence

- `pudl run` now writes the run's terminal verdict to the model instance row
  (`definition = modelTarget(name)` = `"//models/<name>"`, the same key
  `recordModelInstance` ingests under). Mapping (`runVerdict`, `cmd/run.go`):
  converge → `converged` / `failed`; observe-with-desired → `clean` / `drifted`;
  dry-run and pure-populate write nothing. Best-effort: a status-write failure
  never fails the run.
- Surfaced by `pudl model list` (STATUS) and `pudl status` (the read side we kept
  from World A — it's catalog-native, not coupled to the deleted Discoverer).
- **Build-spec §5 fix** (`internal/mubridge/manifest.go`): `ingest-manifest` now
  writes `converging` (not `converged`) on action exit 0 — exit 0 means the apply
  command ran, not that observed==desired. Only the drift re-check writes verified
  `converged`. Removes a latent lie in the loop-less path.

## Shared helpers / refactor

- `modelTarget(name)` (`cmd/run_resolve.go`) — single source of the model
  instance's catalog `definition` key; used by `recordModelInstance`,
  `persistRunStatus`, and `pudl model list`.

## Verification

- `CGO_ENABLED=0 go build ./...`, `go vet ./cmd/...` — clean.
- New `TestRunVerdict` (all branches) passes; DB keying already covered by
  `TestUpdateStatus_Valid` / `TestGetDefinitionStatuses`.
- `CGO_ENABLED=0 go test ./...` — all pass. `model validate` live-tested.

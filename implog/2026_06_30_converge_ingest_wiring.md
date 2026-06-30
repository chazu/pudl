# 2026-06-30 — Wire converge-apply into the per-resource converging→clean lifecycle

## The gap

`pudl run --converge` applied via `mu build` and **discarded the manifest**
(`applyConverge` captured stdout but never used it). So the per-resource
`converging`→`clean` machinery — `IngestManifest(--model)` writing `converging`,
then `PromoteConvergingToCleanByModel` flipping to `clean` on a verified ∅ — was
**only reachable through a separate `pudl mu ingest-manifest` invocation**, never the
main converge path. That's why it was "unit-tested but never observed."

A second, latent half of the gap: the post-run promotion was gated solely on
`report.Drift != nil && report.Drift.Clean`, but the converge path sets
`report.Converge`, never `report.Drift` — so even with `converging` rows present, a
clean converge never promoted them.

## The wiring (commit `cb4db50`)

- **`applyConverge`** now runs `mu build --emit-manifest` (mu routes chatter +
  subprocess stdout to stderr, manifest JSON to stdout) and returns the manifest bytes.
- **`runConvergeLoop`** ingests that manifest after each apply via a new
  `ingestConvergeManifest(modelName, json)` → `IngestManifest(db, …, "mu-build", pudlDir, modelName)`.
  Best-effort: the apply already happened, so an ingest failure warns, not fails.
- **Promotion trigger generalized** (`cmd/run.go`): a verified ∅ from *either*
  `report.Converge` (outcome clean) *or* `report.Drift` (clean) now promotes — so the
  converge loop's final re-observe triggers `promoteConvergingResources`.
- **Action keying fix** (`internal/mubridge/manifest.go`): mu's manifest action carries
  its identifier under **`id`** (`//models/<m>:drift:apply`), and the older `target`
  field is empty — so action rows were getting a blank `target` (filtered from
  `pudl status`, colliding across actions). Now the catalog `target` is keyed off `id`
  when `target` is empty, with a filesystem-safe variant (`/`,`:`→`_`) for the
  stored-action filename.

## Validated live (k8s)

drift (missing) → apply → ingest (`converging`, model-tagged, target
`models/k8s-converge-test:drift:apply`) → re-observe ∅ → promote → `clean`. `pudl status`
shows both the model row (`models/k8s-converge-test`) and the apply-action target as
`clean`. Full `go test ./...` green. This closes the last "not validated against real
infra" item for the declarative-apply class.

## Note on granularity

The k8s plugin emits **one** apply action for the whole manifest, so the per-resource
unit is really per-apply-action (`…:drift:apply`), not per-desired-resource
(`Deployment/nginx`). Finer granularity would need the plugin to emit per-resource
actions — a mu-side concern, not pudl's. The model-tag promotion is robust regardless
(it doesn't depend on the target name).

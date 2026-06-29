# 2026-06-29 — Auto-detect inventory vs differential drift (Tier 1 #1)

Frontier item "run.go dispatch" (`docs/system-models-build-status.md`).

## Problem

A plain `pudl run <model>` routed any model with `desired` to **differential** drift
(`runDrift` — live `mu observe` with desired-as-sources, interpreting per-resource
exists/matches). That's the k8s path. Inventory observers (EweTarget fetchers, the
`host` plugin) emit a flat **record set**, so their output hit the differential parser
unless the user remembered `--from-catalog`. The dispatch bundled two orthogonal axes
(drift computation + observed-state source) behind one flag.

## Change

Drift mode is now auto-detected from the model's observer style:

- **EweTarget populate → inventory** (it produces a record set, by nature).
- **`#PluginObserve` → new `differential` field** (`internal/systemmodel/schema.cue`,
  `differential: bool | *true`): `true` keeps the k8s differential live observe;
  `false` routes to inventory set-diff from the catalog.

Dispatch goes through `useInventoryDrift(model, fromCatalog)` →
`SystemModel.DifferentialDrift()`. `--from-catalog` stays as an explicit override
(force inventory for any model). The `run.go` switch was restructured: the separate
`fromCatalog` case folded into the default, and inventory runs now also evaluate
`checks` (consistent with the differential path).

### CUE default gotcha

`differential` is **required-with-default** (`bool | *true`), not optional
(`differential?: ...`). An unspecified *optional* field stays absent on `inst.Decode`
even with a default, so it would have decoded as Go `false`; a required field with a
default decodes as `true`. EweTarget instances unify with the other union arm (no
`differential`), so requiring it on `#PluginObserve` doesn't break them.

## Scope / API

- `SystemModel.Populate.Differential bool` + `SystemModel.DifferentialDrift()`.
- CLI behavior: inventory observers no longer need `--from-catalog`; the flag is now an
  override. Inventory drift still reads the **catalog** (records pre-ingested via
  populate / `ingest-observe`) — a live-inventory-observe-then-diff path is separate
  and infra-gated, unchanged here.

## Tests

`TestDifferentialDrift` (CUE default → true; `differential:false` → inventory; ewe →
inventory) and `TestUseInventoryDrift` (dispatch matrix). Build + full suite green.

## Tier 1 #2 (harden the converging→clean promotion) — BUILT (exact model-tag path)

Implemented the exact linkage rather than reconstructing resource→model from desired
records:

- `pudl mu ingest-manifest --model <name>` tags each per-action entry with `tags.model`
  (`mubridge.IngestManifest` gained a `model` param; `cmd/ingest_manifest.go` gained the
  flag).
- `CatalogDB.PromoteConvergingToCleanByModel(name)` flips `converging → clean` for all
  rows where `json_extract(tags,'$.model') = name` — exact, regardless of how mu named
  the action targets (`Deployment/web` etc.).
- `promoteConvergingResources` (`cmd/run.go`) tries the exact model-tag path first; if it
  promotes nothing (manifest ingested without `--model`), it falls back to the
  desired-record-name heuristic (`modelResourceDefs`) for back-compat.

Tests (no cluster needed): `TestPromoteConvergingToCleanByModel` (tagged model promotes,
other-model / non-converging / untagged untouched, incl. `Kind/name` defs) and
`TestIngestManifest_ModelTag` (--model tags rows; absent → no tag). **Still to validate
with real infra:** that a live k8s converge + `ingest-manifest --model` + `pudl run`
drift promotes end-to-end (the action-target format is asserted, not yet observed).

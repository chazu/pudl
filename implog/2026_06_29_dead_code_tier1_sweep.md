# 2026-06-29 — §4 Tier 1 dead-code sweep (finish)

Cleared the remaining `docs/vestige-sweep.md` §4 Tier 1 clusters (typepattern was
already done earlier today). Three commits, one cluster each, `deadcode` re-run
between each.

## Clusters removed

1. **`streaming/cue_integration.go`** (commit `1e017a3`) — the `CUESchemaDetector`
   type + ~14 methods; `NewCUESchemaDetector` had zero callers. The parser's
   `SetCUESchemaDetector` uses `NewSimpleSchemaDetector`, not this type. Cleaned two
   stale comments in `schema_detector.go` that named it.

2. **`systemmodel.{LoadModel, LoadModelFile}`** (commit `22e74c7`) — superseded by
   `resolveModel` (`cmd/run_resolve.go`), which loads instances from the schema repo
   and decodes via `DecodeValue`. `LoadModelFile` had zero callers; `LoadModel` was
   used only by `systemmodel_test.go` as a compile-and-decode harness, so it moved
   into the test file as an unexported `loadModel`. Dropped the now-orphaned
   `os`/`cuecontext` imports from the production file.

3. **Legacy base-`Importer` path + `wrapper.go`** (commit `1647f10`) — the big,
   careful one. See below.

## The importer cluster (the careful one)

`cmd/import.go` uses **only** `EnhancedImporter.ImportFileWithFriendlyIDs`; the base
`Importer.ImportFile` path was fully dead in production. **But** every full-pipeline
import test (`importer_test.go`: JSON/YAML/NDJSON routing, k8s/AWS detection,
linux `_schema` collection routing, error cases) ran through that dead path — and the
live content-hash path had **zero** direct test coverage (the cmd-level tests only
cover `resolveFilePaths` globbing and `classifyEnvelopeSchema`). User decision:
**port tests, then delete.**

- **Ported** the full-pipeline tests to drive `ImportFileWithFriendlyIDs` in a new
  `enhanced_importer_test.go`; confirmed green against the live path *before* deleting
  the old code (old file moved aside, new tests run in isolation, all pass incl. the
  NDJSON linux-routing test).
- **Deleted** (all deadcode-unreachable from production):
  - `Importer.{New, ImportFile, GetAvailableSchemas, ReloadSchemas,
    importNDJSONCollection, importWrappedCollection, createCollectionEntry,
    createCollectionItems, createCollectionItem}` + package funcs
    `getValidationStatus`/`getIntendedSchema` (`importer.go`, ~488 lines)
  - `internal/importer/wrapper.go` (whole — `DetectCollectionWrapper` + 11 helpers;
    only caller was the dead `ImportFile`)
  - `metadata.go` file-catalog helpers (`loadCatalog`/`saveCatalog`/`getCatalogDir`/
    `getCatalogPath`) + the orphaned `Catalog` type
  - `schema.go` (contained only `updateCatalog`)
- **Repointed** the only surviving callers (all test scaffolding) to the live API:
  `detection_test.go`, `test/system/config_test.go` → `NewEnhancedImporter`; the
  `test/integration` suite + workflow tests → `NewEnhancedImporter` +
  `ImportFileWithFriendlyIDs`. This made `importer.New` fully dead → also deleted.

### Behavior change

Collection-**wrapper** auto-detection (unwrapping `{"items":[...]}` into a collection)
is **removed** — it existed only in the dead path. The live path handles **NDJSON**
collections (line-delimited records → collection + item entries), not wrapped-array
unwrapping. Since `ImportFile` was already unreachable from the CLI, no user-facing
command lost a feature.

## Net

~1,860 deletions / ~435 insertions across the three commits. `deadcode ./...` now
reports no importer-cluster funcs except the pre-existing Tier-2 orphan
`EnhancedImporter.GetIDDisplayFormat`. `CGO_ENABLED=0 go test ./...` green.

## Still open (per §4)

All Tier 1 clusters are now cleared. **Tier 2** small orphans remain (judgment-call
each): `errors.TUIErrorHandler`, `ui.OutputWriter.{WriteText,WriteLine,Write}`,
`inference/graph.go` accessors, `schemaname` pure utils, assorted singletons, and
`EnhancedImporter.GetIDDisplayFormat`. KEEP: `pkg/` public API, test scaffolding,
`datalog.Compile`.

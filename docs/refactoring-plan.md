# pudl Refactoring Plan: Extract Execution, Simplify Validation

## Motivation

pudl currently has two halves: a **data lake** (import, catalog, schema inference, validation) and an **infrastructure automation layer** (models, methods, Glojure runtime, executor, workflows). The execution layer duplicates what **mu** (a separate graph-based build tool at `~/dev/go/mu/`) already does better and more generally.

This refactoring extracts execution concerns from pudl and into mu, leaving pudl focused on **knowing things**: importing data, validating it against CUE schemas, detecting drift, and exporting action plans for mu to execute.

Additionally, the cascade priority system in the validator is replaced with native CUE unification, and a fixed-point verification system is added to ensure schema inference is idempotent.

## Design Principles

- **pudl owns the "what"** — schemas, catalog, validation, drift detection, data import
- **mu owns the "how"** — when something needs to happen, it's a mu action
- **CUE unification replaces cascade priority** — policy schemas unify on top of base schemas natively
- **Fixed-point property** — re-running inference on all data must produce the same result

## Summary

| Category | Packages/Features |
|----------|------------------|
| **Remove** | `glojure`, `executor`, `workflow`, `model`, `artifact`, `cue/processor` + their CLI commands |
| **Refactor** | `validator` (drop cascade), `inference` (drop priority), `definition` (drop model ref), `drift` (drop executor dep) |
| **Add** | catalog layer, `pudl verify` (fixed-point), `pudl export-actions` (mu bridge), structural validation |

**Net impact:** ~4,700 lines of execution code removed. ~20 internal packages remain (down from ~28).

---

## Phase 1: Delete Execution CLI Commands [DONE]

**Goal:** Remove cmd files that only import execution packages. Each file has its own `init()` registering with cobra — deleting the file cleanly removes the command.

**Delete:**
- `cmd/method.go` (parent command)
- `cmd/method_list.go`
- `cmd/method_run.go`
- `cmd/workflow.go` (parent command)
- `cmd/workflow_run.go`
- `cmd/workflow_list.go`
- `cmd/workflow_show.go`
- `cmd/workflow_validate.go`
- `cmd/workflow_history.go`
- `cmd/model.go` (parent command)
- `cmd/model_list.go`
- `cmd/model_search.go`
- `cmd/model_show.go`
- `cmd/model_scaffold.go`

**Modify:**
- `cmd/repo.go` — remove model discovery from `runRepoValidateCommand()`. Keep definition validation.

**Verify:** `go build ./...` and `go test ./cmd/...` pass. The `pudl method`, `pudl workflow`, `pudl model` commands are gone from the CLI.

---

## Phase 2: Decouple Drift Checker from Execution Layer [DONE]

**Goal:** Rewrite `internal/drift/checker.go` so it no longer imports `executor`, `model`, or `workflow`.

**Current behavior:**
1. Loads definition via `definition.Discoverer`
2. Finds default method on the model via `model.Discoverer`
3. Optionally re-executes the method via `workflow.StepExecutor`
4. Loads latest artifact via `database.CatalogDB.GetLatestArtifact`
5. Compares declared socket bindings vs artifact JSON

**New behavior:**
- Drift = compare a definition's declared CUE values against the latest imported data for a resource_id
- `Checker` takes only `definition.Discoverer` and `database.CatalogDB`
- Remove `Method` from `CheckOptions` and `DriftResult`
- Remove `--refresh` option (execution moves to mu)
- `drift.go` (Compare, FieldDiff) and `report.go` (ReportStore) are unchanged

**Modify:**
- `internal/drift/checker.go` — rewrite to remove executor/model/workflow imports
- `internal/drift/checker_test.go` — update tests for new API
- `cmd/drift_check.go` — simplify `initDriftChecker()` to remove glojure/executor/model/vault setup

**Verify:** `go test ./internal/drift/...` and `go build ./cmd/...` pass.

---

## Phase 3: Remove CUE Processor Glojure Dependency [DONE]

**Goal:** Remove the CUE processor that depends on `glojure.Registry`, and its only CLI consumer.

**Delete:**
- `cmd/process.go` — the `pudl process` command
- `internal/cue/processor.go` — CUE processor with glojure dependency

**Verify:** `go build ./...` passes. The `pudl process` command is gone.

---

## Phase 4: Delete Execution Packages [DONE]

**Goal:** All internal dependents are now unwound. Delete the execution packages.

**Delete entire directories:**
- `internal/glojure/` (~860 lines)
- `internal/executor/` (~990 lines)
- `internal/workflow/` (~1,440 lines)
- `internal/model/` (~630 lines)

**Modify:**
- `go.mod` — remove `github.com/glojurelang/glojure` dependency
- Run `go mod tidy`

**Verify:** `go build ./...` and `go test ./...` pass. Glojure is gone from `go.sum`.

---

## Phase 5: Remove Artifact Package [DONE]

**Goal:** `internal/artifact/` was only imported by files deleted in Phases 1 and 4.

**Delete:**
- `internal/artifact/store.go`
- `internal/artifact/store_test.go`

**Keep in database package:** `ensureArtifactColumns()` and `GetLatestArtifact()` remain — `cmd/data_latest.go` still uses them, and the columns represent historical data.

**Verify:** `go build ./...` and `go test ./...` pass.

---

## Phase 6: Clean Up Bootstrap Schemas [DONE]

**Goal:** Strip execution-era metadata from embedded CUE schemas.

**Modify:**
- `internal/importer/bootstrap/pudl/core/core.cue` — remove `cascade_priority`, `cascade_fallback`, `compliance_level` from `_pudl` metadata in `#Item` and `#Collection`

**Delete:**
- `internal/importer/bootstrap/pudl/model/` (entire directory — model schema + examples)
- `internal/importer/bootstrap/definitions/` (entire directory — example definitions referencing models)

**Verify:** `go test ./internal/importer/...` passes.

---

## Phase 7: Replace Cascade Priority with Native CUE Unification [DONE]

**Goal:** The cascade system reimplements what CUE's type lattice does natively. Replace it.

**Current cascade logic:** Three tiers (policy > base > catchall) sorted by `cascade_priority`, with explicit `cascade_fallback` chains and `compliance_level` tracking.

**New logic:** Try schemas most-specific-first via CUE `Unify().Validate()`. Policy constraints are just CUE definitions that import and tighten base schemas — CUE unification handles the composition. Validation is binary: unifies or doesn't.

**Modify:**
- `internal/validator/validation_result.go`:
  - Remove `CascadePriority`, `CascadeFallback`, `ComplianceLevel` from `SchemaMetadata`
  - Simplify `GetComplianceStatus()` to binary pass/fail

- `internal/validator/cascade_validator.go`:
  - Remove `getCascadeChain()`, `determineCascadeLevel`, `determineFallbackReason`
  - Replace `ValidateWithCascade` with: try intended schema via CUE unification → if fail, try base → if fail, catchall
  - Remove `GetSchemasByResourceType` sorting by priority

- `internal/validator/cue_loader.go`:
  - Remove extraction of `cascade_priority`, `cascade_fallback`, `compliance_level` from `_pudl` metadata

- `internal/validator/validation_service.go`:
  - Simplify `convertCascadeResult` to binary pass/fail

**Verify:** `go test ./internal/validator/...` passes.

**Note on policy schemas:** Policy-violating data is still imported (pudl's "never reject" philosophy). The difference is that validation reports "does not satisfy policy constraints" rather than "fell back to tier 2 with cascade_priority 50." The delta information is preserved — it just comes from CUE's own error messages rather than a bespoke cascade system.

---

## Phase 8: Simplify Inference Graph [DONE]

**Goal:** Remove `cascade_priority` from inference specificity ordering.

**Modify:**
- `internal/inference/graph.go`:
  - Remove `priority` map from `InheritanceGraph`
  - Change `GetMostSpecificFirst()` tiebreaker from cascade_priority to alphabetical (deterministic)
  - Remove `GetPriority()` method

- `internal/inference/graph_test.go` — update tests
- `internal/inference/inference_test.go` — remove `cascade_priority` from test CUE schemas

**Verify:** `go test ./internal/inference/...` passes.

---

## Phase 9: Simplify Definition Package [DONE]

**Goal:** A definition becomes a CUE value conforming to a schema, not a model instance.

**Modify:**
- `internal/definition/definition.go`:
  - Remove `ModelRef` field from `DefinitionInfo` (or rename to `SchemaRef`)

- `internal/definition/discovery.go`:
  - Update regex pattern — definitions unify against schemas directly, not `#*Model` types
  - Update `parseDefinitionsFromFile` accordingly

- `internal/definition/validator.go` — validate against schema types instead of model types

- `cmd/definition_*.go` (list, show, validate, graph) — update display of `ModelRef` → `SchemaRef`

**Verify:** `go test ./internal/definition/...` and `go test ./cmd/...` pass.

---

## Phase 10: Vault Simplification

**Assessment:** The vault package (`internal/vault/`) has zero internal dependencies and a clean interface. Its only execution-layer consumers were deleted in Phases 1-4. The vault CLI commands (`cmd/vault*.go`) only import `config` and `vault`.

**Action:** No changes needed. The vault package and CLI survive as-is. They remain useful for managing secrets that mu plugins will need at execution time.

---

## Phase 11: Add Catalog Layer

**Goal:** A central `catalog.cue` in the schema repo that registers known resource types, schemas, and relationships — inspired by defn's catalog pattern.

**Add:**
- Bootstrap `catalog.cue` in schema path registering core types
- `pudl catalog` CLI command to query/display the catalog
- Documentation for how users add entries to the catalog

**Design:** The catalog is the single inventory that drives schema inference, definition validation, and drift detection. Adding a new resource type means adding a catalog entry; everything else derives from it.

---

## Phase 12: Add Fixed-Point Verification

**Goal:** `pudl verify` re-runs schema inference on all catalog entries and confirms every entry resolves to the same schema. Two-pass idempotency check, inspired by defn's `gen.clj`.

**Add:**
- `cmd/verify.go` — new CLI command
- `internal/verify/` package (or inline in cmd) — iterate catalog entries, re-infer schema, compare to stored assignment

**Semantics:** If `pudl verify` produces differences, it means either:
1. A schema was edited in a way that changes inference results
2. The inference engine has a non-deterministic bug
3. The data was re-imported with different content

All three are worth knowing about. This is a correctness invariant, not a build step.

---

## Phase 13: Add mu Plugin Interface

**Goal:** Bridge pudl's knowledge to mu's execution. `pudl export-actions` reads drift reports and emits mu-compatible NDJSON action DAGs.

**Add:**
- `cmd/export_actions.go` — new CLI command
- `internal/mubridge/` package — translates drift results into mu `ActionSpec` format

**Design:** A mu plugin for pudl would call `pudl export-actions --definition <name>` and parse the NDJSON output as its `plan` response. This keeps the coupling minimal — pudl emits a standard format, mu consumes it.

---

## Phase 14: Add Structural Validation

**Goal:** CUE `close({})` pattern to validate the `~/.pudl/` directory structure.

**Add:**
- A CUE schema defining the expected structure of `~/.pudl/` (data/raw hierarchy, schema packages, catalog.db location)
- Integration with `pudl doctor` to report structural violations

**Inspired by:** defn's `manifest/manifest.cue` which exhaustively validates every file and directory in the repo.

---

## Execution Order & Dependencies

```
Phase 1  ─── Delete execution CLI commands
Phase 2  ─── Decouple drift from execution
Phase 3  ─── Remove CUE processor glojure dep
             │
Phase 4  ─── Delete execution packages (depends on 1, 2, 3)
Phase 5  ─── Delete artifact package (depends on 4)
             │
Phase 6  ─── Clean bootstrap schemas
Phase 7  ─── Replace cascade with CUE unification
Phase 8  ─── Simplify inference graph (depends on 7)
Phase 9  ─── Simplify definitions (depends on 4)
Phase 10 ─── Vault assessment (no changes needed)
             │
Phase 11 ─── Add catalog layer
Phase 12 ─── Add fixed-point verification (depends on 7, 8)
Phase 13 ─── Add mu bridge (depends on drift rewrite in 2)
Phase 14 ─── Add structural validation
```

Phases 1-3 can be done in sequence as a single PR.
Phases 4-5 are a natural second PR.
Phases 6-9 (cascade removal) are a third PR.
Phases 11-14 are independent features, each their own PR.

## Risk Notes

| Phase | Risk | Mitigation |
|-------|------|------------|
| 1 | Cobra command tree breaks | Each cmd file self-registers via `init()` — deleting the file cleanly removes it |
| 2 | Drift checker API change | Only consumed by `cmd/drift_check.go` and tests — small blast radius |
| 4 | Leftover references to deleted packages | Phases 1-3 systematically unwind all importers first |
| 6 | Bootstrap schema changes break importer tests | Run full importer test suite |
| 7 | Validator simplification affects inference | Phase 8 immediately follows to align |
| 9 | Definition discovery regex changes | Definition package has its own test suite |

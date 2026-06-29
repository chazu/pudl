# 2026-06-29 — Remove vendor-specific type-pattern logic (typepattern subsystem)

## Decision

Go should not contain business logic for specific vendor data models / schemas. That
principle applies to **Kubernetes too**, not just the already-dead AWS/GitLab patterns.
So the entire `internal/typepattern` type-detection subsystem — and everything built on
it — was removed.

## Removed

- **`internal/typepattern/` (whole package)** — `pattern.go` (`TypePattern`, `DetectedType`,
  `PudlMetadata`), `registry.go` (`Registry`, `Detect`, ecosystem lookup), and the vendor
  pattern files `kubernetes.go` / `aws.go` / `gitlab.go` (+ all `*_test.go`). The generic
  registry existed only to host vendor patterns, so it went with them.
- **`cmd/schema_generate_type.go`** — the `pudl schema generate-type` command (k8s
  `--kind/--api-version`, aws/gitlab `--ecosystem/--type`). Entirely vendor-type schema
  generation.
- **Importer detect path** (`internal/importer/importer.go`) — `handleUnmatchedData`
  (its only purpose was vendor detection + auto-generating a vendor schema), the helper
  `isCatchall`, and the `typeRegistry` + `schemaGen` fields and their construction. The
  two callers now use the inference result directly (`result.Schema, result.Confidence`).
- **schemagen vendor-import cluster** (`internal/schemagen/generator.go`) —
  `GenerateFromDetectedType`, `generateCUEContentWithImport`, `deriveImportAlias`,
  `WriteSchemaWithSyntaxCheck`, and the now-orphaned `ValidateCUESyntax` /
  `sanitizeIdentifier`; plus the `TestGenerateFromDetectedType` / `TestDeriveImportAlias`
  tests and `test/integration/type_detection_test.go`.
- **Docs** — removed the `generate-type` section from `cli-reference.md` and the
  `typepattern` rows from `architecture.md` and `TESTING.md`. (`VISION.md` / `plan.md`
  left as snapshots per the §3.3 doc-policy.)

## Kept

- `SchemaInferrer.Reload` — its only production caller was the (already-dead) legacy
  `Importer.ReloadSchemas`, but it is exercised by `inference_test.go`. Left in place.
- `formatStringSlice`, `writeSchemaFile`, `WriteSchema` in schemagen — shared, still live.

## Behavior change

Import no longer auto-generates a CUE schema for a detected Kubernetes resource. Data that
inference can't confidently match now simply keeps its inferred (catchall / low-confidence)
result — the same path all other unmatched data already takes.

## Public API delta

- Package `internal/typepattern` no longer exists.
- CLI command `pudl schema generate-type` removed.
- `schemagen.Generator` no longer has `GenerateFromDetectedType` / `WriteSchemaWithSyntaxCheck`;
  package-level `schemagen.ValidateCUESyntax` removed.

## Verification

`CGO_ENABLED=0 go build ./...` clean; `CGO_ENABLED=0 go test ./...` all green. `deadcode`
confirms the removal introduced **no new dead code** (the orphaned schemagen helpers were
removed in the same pass). Recorded in `docs/vestige-sweep.md` §4, which also now carries
the full dead-code assessment (Tier 1/Tier 2) for the remaining clusters (deferred).

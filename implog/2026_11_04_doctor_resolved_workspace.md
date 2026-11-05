# pudl-gvh: doctor checks the resolved workspace, not config.yaml's paths

**Date**: 2026-11-04
**Type**: Bug fix

## Problem

`pudl doctor` built its catalog scan from `loadEffectiveConfig().DataPath`, and
its schema health checks (Schema Repository, Git Repository, Schema Namespace,
Schema Loading, Identity Fields) re-read `schema_path` from `config.yaml`.
Every other command opens the catalog at the resolved workspace root
(`effectivePudlDir()`) and loads schemas from the workspace policy
(`effectiveSchemaPaths`). The global `config.yaml` stores absolute paths, so in
a copied global workspace (`HOME=<copy> pudl --global doctor`) doctor scanned the
original's catalog, against the copy's schemas, and checked the original's
schema directory, reporting bogus "schema not loaded" and secret findings.

## Fix

- `cmd/doctor.go`: the catalog scan uses `effectivePudlDir()`; the health checks
  receive the resolved schema directory (`effectiveSchemaPath`). A new
  "Configuration" health check runs `loadEffectiveConfig()`, so a broken
  `config.yaml` (or, in a repository workspace, paths escaping `.pudl`) is still
  reported now that the schema checks no longer parse the file themselves.
- `internal/doctor`: the schema checks take the schema directory as an argument
  instead of loading `config.yaml`. The no-argument variants (used by tests)
  resolve the global schema directory through `workspace.Global`.

## API changes (internal)

- `doctor.CatalogScan.DataPath` replaced by `PudlDir` (the workspace root whose
  `data/sqlite/catalog.db` is scanned).
- `doctor.CheckSchemaRepositoryAt(schemaPath string)`
- `doctor.CheckGitRepositoryAt(pudlDir, schemaPath string)`
- `doctor.CheckPudlNamespaceSchemasAt(schemaPath string)`
- `doctor.CheckIdentityFieldConsistencyAt(schemaPath string)`
- `doctor.CheckSchemaLoadingAt(schemaPath string)`

No public (`pkg/`) API changed.

## Tests

`cmd/doctor_workspace_test.go` (`TestDoctorChecksTheCopiedGlobalWorkspace`)
copies a global workspace, lets the original gain an extra entry, and runs
`doctor --global --json` with `HOME` at the copy: it must scan one entry and
no health check may mention the original's path. It fails against the old
path resolution.

## Not changed

`pudl list`, `show`, `delete` and `import` still use `cfg.DataPath` (noted in
the issue as behaving the same way; out of scope here). Catalog rows store
absolute `stored_path`s, so a copied catalog still points at the original's raw
payload files.

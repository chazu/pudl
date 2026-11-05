# pudl-orb: list, show, delete and import use the resolved workspace

**Date**: 2026-11-04
**Type**: Bug fix (follow-up to pudl-gvh, `implog/2026_11_04_doctor_resolved_workspace.md`)

## Problem

`pudl list`, `show` and `delete` opened the catalog with
`lister.New(cfg.DataPath)` (catalog at `Dir(data_path)`), and `pudl import`
wrote raw payloads and `.meta` files under `cfg.DataPath`. `data_path` comes
from `config.yaml`, which the global init writes as an absolute path. In a
copied global workspace (`HOME=<copy> pudl --global ...`) list/show/delete
read and wrote the original's catalog, and import recorded its catalog row in
the copy but put the payload in the original's `data/raw`.

The cmd/ audit found the same root cause in:

- shell completions for entry IDs, proquints, origins, relations, sources and
  fact IDs (opened the catalog at `Dir(cfg.DataPath)`);
- `pudl snapshot prune` (`PruneOptions.DataDir = cfg.DataPath`, the boundary
  for raw-file deletion; in a copy it allowed deleting the original's files);
- `pudl run`/`check` populate (`internal/operation/populate.go` passed
  `cfg.DataPath` to `observe.IngestObserve`, so observed payloads went to the
  original);
- `pudl init --global` repair (`internal/init/repair.go` repaired
  `cfg.SchemaPath`/`cfg.DataPath`, writing bootstrap schemas into the
  original's schema directory).

Every other command already used `effectivePudlDir()` and the workspace
policy's schema paths, which ignore `config.yaml`'s paths.

## Fix

One definition of the data directory, `config.DataDir(pudlDir)`
(`<pudlDir>/data`), used everywhere the data directory is needed:

- `cmd/workspace_catalog.go` (new): `effectiveDataDir()`,
  `openWorkspaceCatalog()` and `openWorkspaceLister()`. The openers still run
  `loadEffectiveConfig()` so a broken `config.yaml` (or repository paths
  escaping `.pudl`) is reported as before.
- `list`, `show`, `delete`: `openWorkspaceLister()` replaces the config load
  plus `lister.New(cfg.DataPath)`.
- `import`: the importer's data path is `effectiveDataDir()`.
- completions: `openWorkspaceCatalog()`.
- `snapshot prune`: `DataDir: effectiveDataDir()`.
- `internal/operation`: `Workspace.DataDir()` (from `PudlDir`), used by populate.
- `internal/init` repair: repairs `config.DefaultConfigFor(pudlDir)`'s
  directories (still loading `config.yaml` to report a broken file).

`pudl config` still displays the `schema_path`/`data_path` recorded in
`config.yaml`; no command reads them for catalog, data or schema access any
more.

## API changes (internal)

- `config.DataDir(pudlDir string) string`
- `operation.Workspace.DataDir() string`

No public (`pkg/`) API changed.

## Tests

`cmd/catalog_workspace_test.go` (`TestCatalogCommandsUseTheCopiedGlobalWorkspace`)
copies a global workspace, lets the original gain an entry, then with `HOME` at
the copy: `list --global --json` must not show the original-only entry,
`show --global` of it must fail, `import --global` must leave the original's
catalog unchanged and store the payload under the copy's data directory, and
`delete --global` must remove the entry from the copy only. With the old
resolution all five assertions fail.

The `init --global` repair change has no CLI test: global init/repair fetches a
CUE module.

## Not changed: absolute `stored_path` (follow-up)

Catalog rows still store absolute `stored_path`/`metadata_path`, and `.meta`
files embed both. A copied catalog therefore still reads the original's raw
payloads (`show`, `list --fields`, export, doctor, repair), and `delete`/prune
in the copy refuse to clean those files ("refusing artifact cleanup outside
workspace"), which leaves the copy's own copies orphaned. Making them
workspace-relative is not a small change:

- ~120 references across ~45 files read or write these paths (importer,
  observe, lister, show/list payload loading, export, doctor, repair, scrub,
  sensitive rekey/scrub, payload integrity, prune, bundle, inventory, evidence).
- Reference counting compares exact strings
  (`WHERE stored_path = ? OR metadata_path = ?` in orphan cleanup and prune),
  so absolute and relative forms must never coexist in one catalog.
- `.meta` files carry `stored_path`/`metadata_path` too; `internal/bundle`
  already rewrites both in rows and `.meta` files on restore.

A workable design for the follow-up:

1. Store paths relative to the pudl root (`data/raw/...`, `data/metadata/...`)
   and convert at the `CatalogDB` boundary: relativize in `AddEntry`/`UpdateEntry`
   and the observe/prune/scrub write paths, resolve against `c.configDir` in the
   row scanners. Callers keep seeing absolute paths, so most of the 45 files
   do not change; raw SQL that compares paths (orphan cleanup, prune, scrub) must
   compare the relative form.
2. Idempotent migration: rewrite a row's path to relative when it is absolute
   and has a `data/raw/` or `data/metadata/` suffix whose file exists beneath
   this catalog's root (this also recovers copied catalogs whose rows name the
   original root). Leave other absolute paths (external `DataDir`s, missing
   files) unchanged; the resolver passes absolute paths through, so existing
   catalogs keep working. Already-relative rows are skipped on re-run.
3. Rewrite `stored_path`/`metadata_path` in `.meta` files the same way (or stop
   trusting them in favour of the catalog row), and drop the bundle restore's
   path rewriting once rows are root-relative.

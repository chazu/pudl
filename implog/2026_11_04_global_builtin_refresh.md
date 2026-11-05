# Stale built-ins after a pudl upgrade (pudl-8eh)

## Problem

A workspace keeps copies of pudl's built-in schemas, stamped by
`pudl/.bootstrap-digest`. pudl-8eh (filed 2026-11-02) reported that only
`EnhancedImporter.ensureBasicSchemas` refreshed them, so after an upgrade
whose `#SystemModel` gained `populate.empty_when`, `pudl --global check` and
`pudl --global model validate` failed to load the GCP pack
(`populate: ... errors in empty disjunction`) until an import ran. It also
reported that `pudl init --global` did nothing on an existing `~/.pudl`
although its help says "Initialize or repair".

## Root cause and state of the fix

The single choke point already exists: since e8ec74f (pudl-9vu, 2026-11-03)
the root command's `PersistentPreRunE` calls `refreshBuiltinSchemas`, which
runs `importer.RefreshBootstrapSchemas` on the invocation's primary schema
path (the repository workspace, or `~/.pudl/schema` with `--global`) before
any command loads schemas. That fixed the first half of pudl-8eh, but nothing
tested the issue's scenario (global mode, `#SystemModel` field, model
commands). This task adds that regression test; with the pre-run call
commented out it fails with the issue's error (`#GCPNetworks.populate: 3
errors in empty disjunction`).

The second half was still open: `cmd/init.go` called
`pudlInit.Initialize` for `--global` only when the config was missing or with
`--force`, and `Initialize` returned "already initialized" for an existing
workspace. A built-in deleted or edited while the stamp stayed current was
never restored.

## What changed

- `internal/init/repair.go` (new): `repair(pudlDir, opts)` — for an existing
  global workspace, ensures the schema and data directories and CUE module,
  and rewrites the built-in schemas (`importer.CopyBootstrapSchemas`), keeping
  `config.yaml`. Prints `Repaired PUDL workspace at ... (configuration kept)`.
- `internal/init/init.go`: `Initialize` calls `repair` instead of returning
  when the workspace exists and `Force` is not set. `AutoInitialize` is
  unchanged (it never runs on an existing workspace).
- `cmd/init.go`: `pudl init --global` always calls `Initialize`.
- Docs: `docs/workspace.md` (global mode and `init --global` repair),
  `docs/cli/notes/init.md`, regenerated `docs/cli-reference.md`.

## Public API

No new exported Go API. CLI: `pudl init --global` on an existing `~/.pudl`
now repairs it (previously a no-op), keeping configuration unless `--force`.

## Tests

`cmd/builtin_refresh_global_test.go`:
- `TestGlobalModelCommandsRefreshStaleSystemModel`: global workspace, GCP pack
  installed, `#SystemModel` without `empty_when` and no stamp →
  `pudl --global model validate gcp-fixture-firewalls` succeeds, refreshes
  once, and the file has `empty_when` again.
- `TestInitGlobalRepairsExistingWorkspace`: a deleted built-in with a current
  stamp is restored by `pudl init --global`; `config.yaml` is not rewritten.

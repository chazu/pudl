# Repository-local PUDL persistence

## Summary

Repository workspaces are now complete persistence boundaries. When
`.pudl/workspace.cue` is discovered, CLI commands write configuration, raw
imports, metadata, SQLite catalog rows, facts, snapshots, run reports, and
approval records beneath that repository's `.pudl/`; mutable state no longer
falls through to `~/.pudl/`.
Repository path configuration is validated on load, and `pudl config set`
rejects attempts to redirect `schema_path` or `data_path` outside `.pudl/`.

`pudl repo init` is idempotent and creates or repairs the complete local layout:
the workspace marker, local config, data directories, CUE module, every built-in
schema/rule, `#SystemModel`, model and definition directories, and installed
agent skill. Runtime data and the machine-local absolute-path config are ignored
by the enclosing Git repository.

## Public API

The config package now exposes root-scoped operations for callers that already
own a workspace decision:

- `config.DefaultConfigFor(pudlDir)`
- `config.ConfigPath(pudlDir)`
- `config.LoadFrom(pudlDir)`
- `(*config.Config).SaveTo(pudlDir)`
- `config.ExistsAt(pudlDir)`
- `config.SetConfigValueAt(pudlDir, key, value)`
- `config.ResetToDefaultsAt(pudlDir)`

Doctor checks likewise have `Check*At(pudlDir)` forms, while their original
zero-argument functions retain global-mode compatibility.

## Verification

- Focused config, repo initializer, doctor, and command tests
- Full command-package test suite
- Repeated live `pudl repo init`
- Live local import plus `pudl list --all-workspaces`
- Local `pudl doctor`
- Global catalog hash and modification time unchanged across the live import

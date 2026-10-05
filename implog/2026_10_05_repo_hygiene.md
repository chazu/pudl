# Repository hygiene sweep

**Date:** 2026-10-05

Plan U from the 2026-10-05 improvement review: remove stray files and stale
artifacts, and make the documentation point at one implementation-log location
and the current command surface. No Go behavior changed and no public API was
added.

## Removed

| Path | Why it was dead |
| --- | --- |
| `simple_test.cue`, `test_example.cue` | 2025 scratch files importing a CUE `op` package that does not exist in this repository; nothing referenced them |
| `op/functions.go` | 8-line `CustomFunction` interface; no Go package imported `github.com/chazu/pudl/op` |
| `TODO` | Effectively empty; beads (`.beads/`) is the tracker |
| `ISSUES` | Every item marked DONE; superseded by beads |
| `test/system/catalog/catalog.db-shm`, `catalog.db-wal` | SQLite runtime files committed in `0121b32`; `.gitignore` already excludes them |
| `test/system/cue.mod/module.cue`, `test/system/pudl/core/core.cue` | Bootstrap output from a run inside `test/system`; the system tests build their schema tree under `t.TempDir()` (`config_test.go`) |

The CUE `import "op"` emitted by `pudl model populator new` refers to the ewe/mu
effect package, not the removed Go `op/` directory, and is unchanged.

## Moved

All 40 files under `docs/implog/` moved to the top-level `implog/` with
`git mv`. There were no name collisions and no inbound links to the moved files.
`docs/README.md` now links `../implog/`, and `CLAUDE.md` names the top-level
`implog/` directory and `docs/plan.md` explicitly.

## Documentation

- `FEATURES.md` carries a historical banner pointing at the README command
  table, the CLI reference, and the consolidation mapping.
- `docs/VISION.md` drops the removed Go type-pattern registry, lists current
  commands that were missing (`model new/deps/populator`, `run set`,
  `run report/resume/reject`, `snapshot`, `rule new`, `reclassify`,
  `example install`, `guide`/`prime`, `help --json`), removes a duplicated
  `model validate` line, and recasts future vendor coverage as CUE schema
  packages.

## Tooling

`make clean-local` removes ignored local caches left by manual kick-tires and
walkthrough runs: `.pudl/data/kick-tires/ci-*`, `.pudl/data/kick-tires/bin`, and
`.pudl/data/git-walkthrough`. The smoke tests clean up their own
`test-runs/workspace-*` directories and are unaffected.

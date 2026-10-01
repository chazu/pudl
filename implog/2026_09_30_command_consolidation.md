# Command consolidation

Date: September 30, 2026.

Completed `pudl-0ne`, the bounded command-consolidation slice of the
[UX design report](../docs/design/2026-09-30-ux-simplification-report.md).
Schema/Git and module/CUE helpers remain available. The explicitly registered
top-level command count is reduced from 33 to 28; leaf command paths are reduced
from 76 to 69, excluding help, generated completion, and aliases.

## Public CLI

- `pudl init` creates or repairs local `.pudl/`; `--global` selects `~/.pudl/`.
  Existing local repair and global initialization behavior share one command.
- `pudl schema list` includes catalog metadata, with source and identity details
  under `--verbose` and a complete JSON listing under `--json`.
- `pudl model show` replaces `model describe`, preserves the full model JSON,
  and adds optional `--discover` for Mu plugin capabilities. Model listing and
  inspection use the inherited JSON flag, regardless of argument position.
- `pudl doctor` combines workspace health, assigned-schema validation, and
  fixed-point inference checks. `--entry` selects one catalog record and
  `--health-only` avoids the catalog scan. Explicit producer assignments are
  validated without heuristic reclassification. Findings have a JSON view and
  failures produce a nonzero exit status. Orphan checking now reads the existing
  catalog without creating or migrating it.
- `pudl run set` replaces the separate `run-set` root. Exact-set producer
  selection remains explicit and distinct from standalone recorded-input reuse.
- `pudl run report/resume/reject` handles standalone and set operation IDs.
  Latest report selection compares both persisted report kinds. Named reports
  and decisions route by stored identity, with ambiguous IDs rejected.
  Standalone request-level approvals and set exact-plan approvals retain their
  existing guarantees and JSON payloads.
- `pudl list` shows the active catalog without an implicit origin filter.
  `--origin` remains an explicit filter; the misleading list-only
  `--all-workspaces` flag is removed. Query's separate flag is unchanged.

Former paths are removed rather than maintained as aliases. The root README
contains a migration table. Current guides, embedded skills, initialization
guidance, and executable smoke journeys use the consolidated paths. Help, prime, guide, completion,
and stored-operation inspection avoid global auto-initialization, including when
global flags precede the command.

The memory application, proposed check/apply interface, universal result schema,
direct saved-input checks, and stronger standalone approvals remain outside this
slice. Public Go library APIs and stored evidence are unchanged.

## Validation

- Full ordinary Go tests, vet, build, and embedded-skill synchronization check.
- Focused race tests for `cmd` and `internal/doctor`.
- CLI smoke coverage for command migration, JSON flag placement, local/global
  initialization, schema metadata, retained helper commands, and the installed
  Git inventory walkthrough.
- Real Mu kick-the-tires coverage for dependency ordering, failed producers,
  approvals across processes, stale-plan rejection, sealed routing and outputs,
  standalone provider writes, and concurrent operations.
- Regression coverage for newest standalone/set report selection, ambiguous
  operation IDs, explicit versus inferred record checks, unchanged assignments,
  empty-workspace inspection, and visibility of custom import origins.

The full race suite still fails in the unchanged FastCDC dependency at
`go-cdc-chunkers@v1.0.2/chunkers/fastcdc/fastcdc.go:217` with
`checkptr: pointer arithmetic result points to invalid allocation`. The same
failure was reproduced on original commit `b0b035f` with Go 1.26.2 in an isolated
checkout. It is tracked as `pudl-1ih`. No dependency or streaming changes were
made to mask it.

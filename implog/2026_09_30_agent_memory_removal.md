# Agent memory application removal

Date: September 30, 2026.

Completed `pudl-epk`. Removed the bundled agent-memory application while retaining
the generic bitemporal fact store and Datalog substrate used by models, checks,
and library consumers.

## Removed behavior

- `memory context/init/cycle`, ranked recall, reflection-agent invocation, and
  generated Mu memory targets.
- `hooks print/install/suggest` and their Claude harness integration.
- `facts observe/promote/curate`, observation maturity transitions, feedback
  thresholds, version-lineage curation, and automatic observation/feedback schema
  validation. `facts add --no-validate` is removed with the implicit validation.
- `pull`, the scoped agent-observation recall view.
- `MemoryContext`, the built-in `fact_scored` relation and decay view, and
  `reflect_command` configuration.
- The shipped nous observation/feedback schema, its repository fixture copy,
  the memory guide, and current documentation promoting the application.

The explicit root command count is reduced from 28 to 25 and leaf paths from
69 to 59, excluding help, generated completion, and aliases. Retired commands
are absent from discovery and fail when invoked. The former design plan is
archived under `docs/research/` with an explicit retired status.

## Preserved behavior and migration

Generic fact add/list/show/search/stats/retract/invalidate, temporal history,
transactions, full-text indexing, Datalog, and public library APIs remain.
`facts add` accepts arbitrary JSON objects, with optional explicit CUE schema
validation; its JSON output is now one parseable fact document. Statistics
default to relation counts instead of assuming an observation kind field.

Migration 18 drops only the legacy derived scoring view on the next writable
catalog open. It does not delete or rewrite historical facts, IDs, temporal
bounds, provenance, or search data. Legacy observation and feedback relations
remain ordinary queryable assertions. The old scoring relation name is no
longer reserved by the engine.

Bootstrap repair retires the exact shipped nous schema by content fingerprint.
Authored replacements and symlinks are preserved. No new memory workflow or
schema is installed during initialization.

The installed global `~/.pudl/mu.cue` matched the original default generated
cycle byte for byte. It was retired with a
`mu.cue.memory-retired-2026-09-30` backup. The pristine global nous schema was
also retired with a `nous.cue.memory-retired-2026-09-30` backup. No exact managed
PUDL hooks were found in the checked project and user Claude settings files;
those settings were unchanged. Customized workflows and stored facts are
preserved. The active `/opt/homebrew/bin/pudl` symlink points to `~/go/bin/pudl`,
the project installation target.

Current prime/guide output and canonical, embedded, and linked repository skills
describe model operations and generic evidence queries. The UX report, README,
CLI reference, fact documentation, and development plan record the retirement.

## Validation

- Ordinary full-workspace tests, vet, build, and embedded-skill synchronization.
- Focused race tests for commands, database, Datalog, importer, config, and repo
  initialization. The previously recorded FastCDC full-race failure remains
  outside this change (`pudl-1ih`).
- Migration regression proving historical and current facts, IDs, and full-text
  search survive retirement of the scoring view.
- Bootstrap repair regressions proving pristine retirement, repeated repair,
  preservation of authored changes, and preservation of symlinks.
- CLI smoke coverage proving retired commands and guides fail, fresh workspaces
  do not install memory assets, and generic JSON writes, search, statistics,
  retraction, and historical inspection remain usable.
- Installed Git inventory walkthrough and real Mu kick-the-tires coverage for
  dependency ordering, exact approvals, stale-plan rejection, sealed routing,
  producer/consumer execution, and concurrent operations.

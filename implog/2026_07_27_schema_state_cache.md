# Compile schema state once per invocation (Recommendation 6)

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-schema-state-cache.md`
**Closes:** Recommendation 6 from `docs/architecture-improvement-report.md`

## What was wrong

`validator.CUEModuleLoader.LoadAllModules` is the expensive primitive under every
consumer of schema state: it parses and CUE-compiles every module in a directory,
and on a missing dependency shells out to `cue mod tidy` and retries. Every
consumer built its own. A single `pudl run` compiled the same unchanged schemas
at least twice — once for inventory identity resolution, once for the populate
ingest — and `resolveModel` ran `listModelsIn` once per model search directory,
each a fresh compile.

## What changed

**Two levels of memoization, both invalidated by a file fingerprint.**

- `validator.SharedLoader(path)` returns a process-wide loader per absolute
  path, and `LoadAllModules` memoizes its own result. Two callers naming the same
  directory share one compile; one caller loading twice compiles once.
- `inference.Shared(paths...)` memoizes the assembled inferrer — the merged
  schema and metadata maps and the inheritance graph built from them, which the
  loader memo alone would leave rebuilt on every call.

**Fingerprint, not just path.** A path-keyed memo would be wrong *within* one
invocation, not merely stale: `pudl schema new` writes a schema and then infers
against it. The fingerprint hashes every file under the path by relative path,
size and modification time — one `stat` per file, against a CUE compile.

**The loader is shared, not just its modules.** A loader owns a `cue.Context`,
and CUE values from different contexts cannot safely be unified. Caching modules
while callers kept private contexts would hand a caller values built in someone
else's context — a hazard the uncached code does not have. Sharing per path also
reduces the number of contexts in play rather than increasing it.

**Returned maps are copied**, so a caller writing into what it was handed cannot
poison later readers. `cue.Value` is immutable, so only the map spines need it.

**A failed load is never cached.** That is exactly when the caller may have fixed
the problem — a dependency fetched, a syntax error corrected — between attempts.

## Public API

- `internal/validator`: `SharedLoader`, `ResetSharedLoaders`, `ModuleLoadCount`,
  `(*CUEModuleLoader).Fingerprint`. `LoadAllModules` is unchanged in signature and
  behaviour, now memoized.
- `internal/inference`: `Shared`, `ResetShared`. `NewSchemaInferrer` unchanged and
  still unshared, for callers needing isolation.
- Call sites moved to the shared forms: the inferrer (7 command sites plus the
  importer), the chain validator (2), `listModelsIn`, `cmd/catalog.go`.

## Tests

`internal/validator/module_cache_test.go` — a second load is served from the
memo; a changed, added or removed file recompiles; two callers share one compile;
distinct paths are distinct loaders; a relative and an absolute path resolve to
the same loader; a caller mutating the returned maps cannot poison the next
reader; a failed load is retried rather than served.

`internal/inference/shared_test.go` — the same inferrer for unchanged paths; a
changed schema rebuilds *and the rebuilt inferrer sees the new schema*; path
order is part of the key (shadowing is first-found-wins); three callers cause one
CUE compile; `NewSchemaInferrer` stays unshared.

Both packages pass under `-race`, which is what the mutex is for: nothing in pudl
loads schemas concurrently today, but a memo that is not race-safe is a trap for
the first caller who does.

## Known limit

Modification time is not a perfect change signal: a same-nanosecond write of
different content within one invocation would be missed. Size is in the hash too,
and the write would have to come from pudl itself between two loads in the same
process. Hashing contents would make the check cost proportional to the schema
repo on every lookup, which is the cost the cache exists to avoid.

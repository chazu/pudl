# Design: compile schema state once per invocation

**Date:** 2026-07-27
**Implements:** Recommendation 6 from `docs/architecture-improvement-report.md`
**Status:** stabilized after adversarial review (§6); ready to execute

## 1. What is recompiled

Every consumer of schema state builds its own. `validator.CUEModuleLoader` is the
common primitive underneath all of them, and `LoadAllModules` is the expensive
call: it parses and CUE-compiles every module in a schema directory, and on a
missing dependency it shells out to `cue mod tidy` and retries.

| Caller | Constructs |
|---|---|
| `inference.NewSchemaInferrer` | one loader per schema path, every time |
| `validator.ChainValidator` (×2 sites) | one loader per schema path |
| `cmd/model_list.listModelsIn` | one loader per model search dir, per call |
| `cmd/catalog.go` | one loader |

A single `pudl run` builds a `SchemaInferrer` at least twice — once in
`schemaIdentityResolver` for inventory identity, once in the populate ingest —
and `listModelsIn` runs once per search directory inside `resolveModel`. Each is
a full compile of the same unchanged files.

## 2. The cache

**Memoize at the loader, and share loaders by path.**

```go
func SharedLoader(schemaPath string) *CUEModuleLoader   // memoized per absolute path
func (l *CUEModuleLoader) LoadAllModules() (map[string]*LoadedModule, error) // memoized per loader
```

Two levels, both needed:

- Per-loader memoization means a caller that loads twice compiles once, even
  with a private loader.
- `SharedLoader` means two *callers* naming the same path share the compile.

### 2.1 Keyed by a file fingerprint, not just the path

A path-keyed memo would be wrong within a single invocation, not merely stale:
`pudl schema new` writes a schema and then infers against it, and
`pudl schema reinfer` does the same. Serving the pre-write compile there would
be a correctness bug introduced by an optimization.

The fingerprint is a hash over every file under the schema path: relative path,
size, and modification time in nanoseconds, walked in a deterministic order.
Recomputing it is one `stat` per file — cheap next to CUE compilation, and cheap
next to a `cue mod tidy` subprocess.

### 2.2 Sharing the loader, not just the modules

The loader owns a `cue.Context`, and CUE values from different contexts cannot
be safely unified. Caching *modules* while callers kept private contexts would
hand a caller values built in someone else's context — a hazard the current code
does not have, introduced by the cache.

Sharing the whole loader keeps context and values together. It also *reduces* an
existing hazard: `inference.loadSchemasFromPaths` already builds one context per
schema path and merges the results, so schema values from different paths
already live in different contexts. Sharing per path does not change that; it
just stops multiplying contexts per call.

### 2.3 Returned maps are copied

`LoadAllModules` returns `map[string]*LoadedModule`, each holding two maps. A
caller writing into one would poison every later reader. The maps are shallow-
copied on return — `cue.Value` is immutable, so only the map spines need it — so
"cached" stays invisible to callers. The copy is O(number of schemas) map
inserts against a CUE compile; it does not register.

### 2.4 Concurrency

Guarded by a mutex. Nothing in pudl loads schemas concurrently today, but a
package-level memo that is not safe under `go test -race` with parallel tests is
a trap waiting for the first caller who does.

## 3. Blast radius

| Surface | Change |
|---|---|
| `internal/validator` | `SharedLoader`, per-loader memo, fingerprint, `ResetSharedLoaders` for tests |
| `internal/inference` | uses `SharedLoader` |
| `cmd` | `listModelsIn`, `catalog.go` use `SharedLoader` |
| behaviour | none: same modules, same errors, same `cue mod tidy` on a cold load |

## 4. Tests

| Case | Assertion |
|---|---|
| Two loads, unchanged files | compiled once (observed via a load counter) |
| A file changes between loads | recompiled |
| A file is added | recompiled |
| A file is removed | recompiled |
| Two callers, same path | same loader, one compile |
| Two callers, different paths | different loaders |
| Caller mutates the returned map | the next caller is unaffected |
| Load error | not cached as success; retried |
| `ResetSharedLoaders` | drops the memo |

## 4b. A second level: the inferrer itself

*Added during implementation.* Memoizing the loader shares the CUE compile, but
`NewSchemaInferrer` also merges schema and metadata maps with first-found-wins
shadowing and builds an inheritance graph from them — and the recommendation
names all three ("schema loading, CUE compilation, inheritance graphs, and
identity metadata are reconstructed repeatedly"). The loader memo alone would
leave the last two rebuilt on every call.

`inference.Shared(paths...)` memoizes the assembled inferrer, keyed by the path
list **in order** (shadowing is first-found-wins, so `[a b]` and `[b a]` are
genuinely different) and invalidated by the concatenation of the same loader
fingerprints — not a second signal that could disagree with theirs. A path whose
fingerprint cannot be computed contributes a sentinel, so an unreadable
directory always looks changed and never serves a stale inferrer.
`NewSchemaInferrer` stays as the escape hatch for a caller that needs isolation.

## 5. Non-goals

- No cross-invocation (on-disk) cache. The report asks for "within a command
  invocation"; persisting compiled CUE is a different problem with a different
  invalidation story.
- No change to which schemas win when paths shadow each other. That ordering is
  the workspace policy's, settled in Recommendation 5.

## 6. Adversarial review

**A1 — "Modification time is not a reliable change signal."** *Accepted with the
scope stated.* A same-nanosecond write of different content within one
invocation would be missed. Filesystems with coarse timestamps make that
theoretically reachable, but the write would have to come from pudl itself
between two loads in the same process, and size is in the hash too. Hashing file
*contents* would make the check cost proportional to the schema repo on every
lookup, which is the cost the cache exists to avoid. Recorded rather than
mitigated.

**A2 — "A package-level memo is global mutable state."** *Accepted, with the
alternative rejected.* Threading a cache object through `NewSchemaInferrer`,
`ChainValidator`, `listModelsIn` and every command would put a parameter in a
dozen signatures to serve a process-lifetime concern. The memo is keyed by
absolute path, guarded by a fingerprint, mutex-protected, and resettable. What
would make it dangerous is unbounded growth, and it is bounded by the number of
distinct schema paths an invocation names — two or three.

**A3 — "Copying the maps defeats the point."** *Rejected on the arithmetic.* The
copy is map inserts proportional to the schema count; the thing avoided is CUE
parse+compile of every module, plus a possible `cue mod tidy` subprocess. These
are not the same order of magnitude.

**A4 — "Sharing a `cue.Context` across callers could surface CUE-internal state
sharing."** *Considered, and the direction is toward safety.* A context is
designed to be shared — it is CUE's own value-interning scope, and unifying
values from *different* contexts is the documented hazard. This change reduces
the number of contexts in play, it does not increase it.

**A5 — "An error should be cached too, or a broken schema dir is re-compiled on
every lookup."** *Rejected.* A failed load is exactly the case where a caller may
have fixed the problem (`cue mod tidy` fetching a dependency, a syntax error
corrected by `pudl schema new`) between attempts. Caching the failure would make
the fix invisible until the next process. The cost of retrying a failing load is
paid only by an already-broken repository.

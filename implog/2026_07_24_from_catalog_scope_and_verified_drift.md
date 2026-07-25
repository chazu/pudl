# `--from-catalog` scoping + verified drift (Defect 3 / pudl-qp6)

Fixed the P0 in the defect register: `--from-catalog` ran with no scope and its
verdict was trusted as though it were a live observation, so a replay could
report `clean` off another model's records and promote `converging` resources on
the strength of it.

## The defect

Two compounding bugs on the catalog-replay path.

**3a — no scope was applied.** `var scope string` was only assigned inside
`if !flags.fromCatalog`, so the replay path called `runInventoryDrift(db, "", …)`.
The `if scope != ""` guard in `loadObservedRecords` then left both `filter.Origin`
and `filter.CollectionID` empty, and `QueryEntries` returned every
`entry_type='observe'` item row in the catalog — every model, every host, every
snapshot, all time. A desired record could be satisfied by an observation from an
unrelated model or host sharing schema and identity, at any age.

**3b — a replay verdict was trusted.** `verifiedClean` accepted any
`report.Drift.Clean`, including the replay path, and drove both
`persistRunStatus(…, "clean")` and `promoteConvergingResources`. Running
`pudl run m --from-catalog` right after an apply flipped every resource that
apply had left `converging` to `clean`, off a set-diff against records that may
predate the apply.

## Why the scope is explicit rather than inferred

The obvious fix — scope to the model's own populate target — does not work.
Observe items do carry a `target` column, and a model's populate target is
`models/<name>:populate`, but records ingested through `pudl ingest-observe`
carry whatever target their observer reported (the existing inventory test uses
`//host:odroid`). Inferring a model's records from its target would therefore
break the externally-ingested case that `--from-catalog` exists to serve, and
report every desired resource as `missing`.

Snapshots are also not target-filterable: `createObserveSnapshot` writes no
`Target` on the snapshot's own collection entry, so "the latest snapshot for this
model" is not directly derivable either. Making it derivable is Recommendation 3
(first-class scoped snapshots), not this fix.

So the scope is requested, not guessed. `--from-catalog` now requires
`--catalog-scope`, and an unscoped replay is refused before any query runs.

## Public API

`pudl run` gains one flag:

- `--catalog-scope <snapshot-id|origin>` — which already-ingested records
  `--from-catalog` replays. Required with `--from-catalog`; an error without it.
- `--catalog-scope` without `--from-catalog` is also an error.

`acute.ModelDriftResult` gains one field:

- `Verified bool` (`json:"verified"`) — whether the verdict came from a fresh
  observation of the live system. Defaults to **false** so that a path which
  forgets to claim verification stays untrusted rather than silently
  authoritative.

Behaviour change: a `--from-catalog` run no longer writes a model status in
either direction, and never promotes `converging` resources. The model keeps the
verdict of its last real observation, which is correct — the replay observed
nothing.

## Implementation

- `internal/acute/drift.go` — `Verified` on `ModelDriftResult`.
- `cmd/run_drift.go` — `interpretDifferentialObserve` sets `Verified: true`; a
  differential observe always reads the live system.
- `cmd/run.go` — `catalogScope` on `runFlags`; `validateRunFlags` enforces the
  pairing; the inventory branch takes its scope from the flag on the replay arm
  and from the fresh snapshot otherwise, and errors if populate yields no
  snapshot; `res.Verified = !flags.fromCatalog`; `verifiedClean` and `runVerdict`
  both require `Verified`.
- `cmd/run_inventory.go` — `runInventoryDrift` refuses an empty scope
  independently of its caller, so the invariant is local rather than inherited.
  Parameter renamed `origin` → `scope` to match what it now accepts.

## Tests

- `cmd/run_test.go` — scope-pairing validation (missing, blank, both, scope
  without the flag); `runVerdict` cases proving a replayed drift writes nothing
  clean *or* dirty, and that verified drift still writes both.
- `cmd/run_inventory_test.go` — empty-scope refusal; cross-scope isolation
  (records ingested under one origin do not satisfy another scope's desired
  state, and do satisfy their own); `Verified` is false out of
  `runInventoryDrift`.

`CGO_ENABLED=0 go test ./...` and `go vet ./...` pass.

## Follow-ups left open

`pudl-sln` is adjacent: `loadObservedRecords` still treats a genuine
`GetCollectionByID` error the same as not-found and falls back to an origin
filter, so a DB error can still yield a confident "everything is drifted".
Scoping is now mandatory, which bounds the damage, but the error path is still
wrong.

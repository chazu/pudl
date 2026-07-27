# One run-owned catalog handle

Closes `pudl-n6j`. Recommendation 4's remaining lifecycle slice, after the step
transaction boundary (`pudl-xlx`, `implog/2026_07_25_catalog_step_transactions.md`).

A `pudl run` now opens the catalog at most once and closes it once. It used to
open it eleven times.

## What eleven opens cost

`cmd/run.go` ×5 (the inventory branch, `persistRunStatus`, `startRunRecord`,
`finishRunRecord`, `promoteConvergingResources`), `run_checks.go`,
`run_depends.go` ×2, `run_populate.go`, `run_converge.go`, `run_drift.go`.

Every `NewCatalogDB` re-runs `createTables` and all migrations — idempotent by
design, so this was waste rather than breakage: eleven schema setups and eleven
independent lifecycles per run, each with its own `defer db.Close()` to get right.

The converge loop made it worse than the count suggests. `recordDriftObservation`
and `ingestConvergeManifest` are both inside the loop, so a five-iteration
converge opened and closed the catalog on every pass.

## `runCatalog` (cmd/run_catalog.go)

The handle is created in `RunE` and threaded to every phase that touches the
catalog. `defer cat.Close()` is registered before the run-record finalizer, so
LIFO puts the close after `finishRunRecord` — the audit row is still written on
every exit path, including an early `return err`.

**Opening is lazy and memoized.** Nothing opens until a phase borrows, and the
outcome (success *or* failure) is remembered. Two reasons:

- It keeps `--dry-run` honest. A dry run promises no catalog writes and today
  reaches no catalog-touching phase; eager opening would have made it create
  `data/sqlite/catalog.db` and run every migration. Verified by hand: `pudl run
  nosuchmodel` under a fresh HOME leaves `.pudl/data/` empty, with no `sqlite/`
  subdirectory at all.
- Memoizing the *failure* means a broken catalog is diagnosed once per run rather
  than re-attempted by each of the six best-effort phases in turn.

## The distinction the ticket warned about

`pudl-n6j` flagged the real hazard: opening in place let each phase inherit its
failure semantics from its call site. The inventory, checks and populate paths
treat "cannot open the catalog" as fatal; the status write, both halves of the
run record, the dependency reconcile, promotion and the drift observation treat
it as a warning and must never fail the run. Sharing one handle erases that
context, so a borrow now has to name which it is:

```go
func (c *runCatalog) required() (*database.CatalogDB, error)  // err fails the run
func (c *runCatalog) optional() (*database.CatalogDB, error)  // nil db => return, don't fail
```

`required` wraps the error as `open catalog: %w`, the way its callers already
returned it. `optional` hands the open error back rather than swallowing it, so a
dropped write can still be reported — best-effort is not the same as silent, and
a status write that vanishes leaves the previous run's verdict standing, which is
how a stale `clean` used to survive.

Existing warning behaviour is preserved verbatim, including the two callers that
deliberately stay quiet. Both now say so where they ignore the error:
`finishRunRecord` (its `startRunRecord` borrowed the same handle and already
warned, and there is no row to finish anyway) and `checkUpstreamFreshness` (a
read-only advisory documented to return no warnings on any failure).

## Signature churn

Mechanical, as anticipated. `*runCatalog` is now the first parameter of
`persistRunStatus`, `startRunRecord`, `finishRunRecord`,
`promoteConvergingResources`, `runChecks`, `reconcileModelDependencies`,
`checkUpstreamFreshness`, `recordModelInstance`, `runPopulate`, `runEwePopulate`,
`ingestObserveOutputWithSnapshotRunID`, `ingestConvergeManifest`,
`runConvergeLoop`, `setupReconcileWorkspace`, `runDrift` and
`recordDriftObservation`. All are unexported and confined to the run path.

`reconcileWorkspace` gained a `Catalog` field instead of a parameter, because
`observeDrift` is a method called once per converge iteration. The field's comment
states that the workspace borrows and must not close it.

## Two comments that stopped being true

- The inventory branch had a note explaining that the catalog is opened *after*
  populate, because opening up front left a reader handle open across
  `runPopulate` — which opened its own handle and wrote the very records the
  reader then read. There is one connection now, so the hazard is gone; the
  comment says that instead.
- `ingestConvergeManifest` took `pudlDir` from `config.GetPudlDir()` separately
  from its catalog open. It now uses `cat.Dir()`, so the manifest's raw files and
  its catalog rows cannot land under different roots. Same for
  `checkUpstreamFreshness`'s rule search path.

## Public API

None. `runCatalog` is unexported and lives entirely in `package cmd`; no
`internal/` or `pkg/` signature changed.

```go
// cmd — new, unexported
type runCatalog struct{ /* ... */ }

func newRunCatalog(pudlDir string) *runCatalog
func (c *runCatalog) Dir() string
func (c *runCatalog) required() (*database.CatalogDB, error)
func (c *runCatalog) optional() (*database.CatalogDB, error)
func (c *runCatalog) Close() error
```

## Tests

`cmd/run_catalog_test.go`:

- `TestRunCatalog_LazyOpen` — constructing and closing the handle creates no
  `data/sqlite/catalog.db`. This is the `--dry-run` guarantee.
- `TestRunCatalog_OneOpenServesEveryBorrow` — `required` and `optional` return the
  same `*CatalogDB` pointer; the first borrow is what opens it.
- `TestRunCatalog_RequiredFailsWhereOptionalCarriesOn` — against a pudl dir that
  cannot hold `data/sqlite/` (a regular file in the way), `required` errors with
  `open catalog` and `optional` returns a nil handle plus the error.
- `TestRunCatalog_FailedOpenIsNotRetried` — unblocking the path after a failed
  borrow does not let a later borrow succeed. A retrying handle would give one run
  two different catalogs.
- `TestRunCatalog_DirIsThePudlDirItWasGiven`.

`TestScopedCleanRun_ModelRowUnknown_RunRowKeepsVerdict` and
`TestUnscopedCleanRun_ModelRowClean` now drive one handle across the status write
and both halves of the run record, matching what `pudl run` does.

Full suite green (`CGO_ENABLED=0 go test ./...`), `go vet ./...` clean. The
`smoke`-tagged tests were not run — they need docker/k3d/kubectl/mu.

## Still outstanding from Recommendation 4

Unchanged by this slice, and not filed as tickets: an explicit migration version
table, and retiring the legacy collection columns in favour of
`collection_memberships`.

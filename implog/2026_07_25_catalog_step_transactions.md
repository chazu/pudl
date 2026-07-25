# One catalog transaction boundary per recorded step

Closes `pudl-xlx`. Recommendation 4's first slice: the catalog can now record a
step atomically, and both steps that record one do.

## The unit is the step, not the run

The ticket title says "no shared transaction boundary within a run", but a
run-spanning transaction is the wrong thing to build and was deliberately not
built. A converge run shells out to mu for minutes at a time; a write transaction
held across that would lock the catalog for the whole run — against other pudl
invocations and against the run's own other handles.

What actually wants to be atomic is each *step* that records a result:

- an **observation** — the snapshot entry plus every membership that makes it
  mean anything;
- a **convergence step** — the manifest, its per-action entries, and the statuses
  those actions imply.

Both are short and hold no lock across a subprocess.

## What a partial step used to leave

Neither step was atomic, and both failure modes are states a later run reads as
truth:

- A failed observe ingest left a snapshot describing records that were never
  stored, or records belonging to a snapshot that does not exist. Inventory drift
  reads exactly that.
- A failed manifest ingest left an apply on record whose resources still read
  `unknown`. This is the hole `repairMissingActionStatuses` (added the day before,
  for `pudl-6pf`) exists to patch up after the fact — the status write was a
  `fmt.Fprintf(os.Stderr, "Warning: ...")` that execution walked straight past.

Inside a transaction, warning past a failed status write would commit the very
hole it was papering over, so that write now aborts the step.

## Shape

`CatalogTx` mirrors the `FactTx`/`WithFactTx` idiom already in the package rather
than inventing a second one: `BEGIN IMMEDIATE` on a dedicated connection, rollback
on error, commit otherwise, reads seeing the transaction's own writes.

`CatalogWriter` is the operation set a step performs, satisfied by both
`*CatalogDB` (each call its own implicit transaction) and `*CatalogTx` (all calls
in one). The ingest paths take the interface, so they are written once and run
either way, and the exported mubridge API is unchanged.

Each operation a step needs gained an executor-parameterized core — `addEntryIn`,
`addCollectionMembershipIn`, `updateStatusIn`, `getEntryIn`,
`findByContentHashIn`, `getLatestObserveByContentHashIn`, `getTargetStatusesIn` —
with the `CatalogDB` method delegating to it. Same pattern as `addFactIn`.

## Row mapping, centralized

Running the same reads against a transaction meant the column list and Scan had
to stop being copy-pasted. `catalog_rows.go` now holds `entryColumns`, `scanEntry`
and `scanOptionalEntry`; a dozen sites had hand-maintained duplicates of both.
This is the footgun CLAUDE.md names outright — "all SQL SELECT/Scan operations
must be kept in sync when adding columns" — reduced to one edit. `rowScanner`
already existed in `catalog_runs.go` and moved here.

## Side effect worth naming

The manifest dedup check now runs inside the transaction that acts on it, which
closes a race it had on its own: two concurrent ingests of the same manifest could
both find nothing and both insert. `BEGIN IMMEDIATE` takes the write lock before
the check, so the second sees the first's manifest.

## Known cost

The write lock is held for a whole step rather than per row, and the observe
ingest stages its raw files inside that window, so a concurrent writer waits for
the batch rather than interleaving with it (bounded by the 5s busy_timeout). For
a normal observation this is tens of milliseconds. A step large enough to
approach the timeout wants its file staging hoisted out of the transaction —
a change to the ingest loop, not to this boundary. Documented on `WithCatalogTx`.

Rollback covers catalog rows only. Raw files staged during a step survive it;
they are content-addressed and inert without a row pointing at them, so an
abandoned step leaves unused bytes rather than a corrupt catalog.

## Behaviour change

`IngestObserveResultsWithSnapshotRunID` used to return a partial count and the
snapshot ID on error. Both described rows that had just been rolled back, so it
now returns `(0, "", err)`.

## Not in this change

Two parts of Recommendation 4 remain, filed as `pudl-n6j`:

- **One run-owned handle.** The run path opens the catalog eleven times, and every
  `NewCatalogDB` re-runs `createTables` and all migrations. Worth doing, but it is
  lifecycle work rather than atomicity work and carries broad signature churn
  across the run path — bundling it here would mix a correctness fix with a large
  mechanical refactor. Those opens are also what currently makes the best-effort
  "a missing catalog never fails the run" semantics work, so a shared handle has
  to carry that explicitly rather than inherit it by accident.
- An explicit migration version table, and retiring the legacy collection columns.

## Public API

```go
// internal/database — new
type CatalogWriter interface {
	AddEntry(entry CatalogEntry) error
	AddCollectionMembership(collectionID, itemID string, itemIndex int) error
	UpdateStatus(targetName, status string) error
	GetEntry(id string) (*CatalogEntry, error)
	FindByContentHash(contentHash string) (*CatalogEntry, error)
	GetLatestObserveByContentHash(targetName, contentHash string) (*CatalogEntry, error)
	GetTargetStatuses() ([]TargetStatus, error)
}

type CatalogTx struct{ /* ... */ }        // implements CatalogWriter
func (c *CatalogDB) WithCatalogTx(fn func(*CatalogTx) error) error
```

`*CatalogDB` keeps every method it had, with identical signatures and semantics.

## Tests

- `TestWithCatalogTx_CommitsOnSuccess` / `_RollsBackOnError`.
- `TestWithCatalogTx_EntryAndMembershipRollBackTogether` — an entry and the
  membership `AddEntry` writes for it roll back as a pair, so no membership can
  point at a row that does not exist.
- `TestCatalogTx_ReadsSeeUncommittedWrites` and
  `TestCatalogTx_ContentHashLookupsSeeUncommittedWrites` — without this every
  check-then-write inside a step (dedup, status repair) silently stops working.
- `TestIngestManifest_StepIsAtomic` — forces a real mid-step failure with two
  byte-identical actions (same content-addressed ID, so the second collides) and
  asserts nothing at all is recorded. Verified non-vacuous: without the
  transaction the same test finds 2 entries and a `converging` status.

Full suite green (`CGO_ENABLED=0 go test ./...`), `go vet ./...` clean. The
`smoke`-tagged tests were not run — they need docker/k3d/kubectl/mu.

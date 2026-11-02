# pudl-dx4: byte-identical records from two collectors no longer fail ingest

## Bug

When a second collector (another model, or another target in the same
observation) produced a record byte-identical to one already stored, observe
ingest failed with `UNIQUE constraint failed: catalog_entries.id` and the whole
observation was rolled back.

## Root cause

An observe record's entry ID is its content hash alone
(`prepareObserveRecord`, `internal/observe/ingest.go`). The dedup check in
`PreparedObservation.Commit` (`internal/observe/prepare.go`) looked the record
up with `GetLatestObserveByContentHash(target, contentHash)`, which is scoped to
the collector's target. A byte-identical record reported under a different
target therefore missed the lookup and went on to `AddEntry` with an ID that
already existed. The check did run inside the commit transaction; the problem
was only the lookup key.

## Fix

`Commit` now looks the record up by its entry ID (`tx.GetEntry(entry.ID)`,
treating `ErrCodeNotFound` as "not stored yet"), the same key the catalog
enforces as unique. When the entry exists it takes the existing dedup path: it
fills in a missing identity, adds a membership in the new snapshot, and
re-observes facts when the schema matches. So the second model's snapshot
includes the shared entry, and a complete snapshot does not treat that resource
as gone.

The stored entry keeps the `target` of the first collector that reported it.
Snapshot contents are read from `collection_memberships`, so this does not
affect what a snapshot contains.

## Public API changes

None. `CatalogDB.GetLatestObserveByContentHash` and the `CatalogTx` /
`CatalogWriter` method of the same name are left in place, but no production
code calls them any more. Only their tests do.

## Net production line delta

+8 / -0 net (`internal/observe/prepare.go`: +9, -1; 4 of the added lines are
comments).

## Validation

- New tests in `internal/observe/shared_record_test.go`:
  - `TestIngestObserve_SecondCollectorReusesByteIdenticalRecord`: two models,
    two complete snapshots, different targets, byte-identical record. The
    second ingest succeeds and stores 0 new records. Both snapshots contain the
    same entry, which has 2 memberships.
  - `TestIngestObserve_ByteIdenticalRecordsAcrossTargetsInOneBatch`: two
    targets in one observation share one entry.
  - Before the fix, both failed with the UNIQUE constraint error.
- `CGO_ENABLED=0 mise exec -- go test -count=1 ./...`: all packages pass.
- `CGO_ENABLED=0 mise exec -- make lint`: 0 issues.

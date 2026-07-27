# Design: a migration version table, and memberships as the sole collection source

**Date:** 2026-07-27
**Implements:** Recommendation 4's two outstanding items
**Status:** stabilized after adversarial review (§6); ready to execute

## 1. What is left of Recommendation 4

Two slices shipped: `CatalogTx`/`WithCatalogTx` (one transaction per recorded
step) and the run-owned catalog handle. The recommendation still asks for:

> It should also ... introduce an explicit migration version table. Once callers
> no longer depend on the legacy collection columns, those columns can be retired
> in favor of `collection_memberships` as the sole relationship source.

## 2. Part one: a migration version table

Today every `NewCatalogDB` re-runs every schema step, each written to be
idempotent by construction: `CREATE TABLE IF NOT EXISTS`, `columnExists` guards,
`CREATE INDEX IF NOT EXISTS`. It works, but:

- correctness rests on every author remembering to write the guard;
- a step that *cannot* be expressed idempotently has nowhere to go;
- there is no record of what a given database has actually had applied to it,
  so a partially-migrated database is indistinguishable from a current one.

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMP NOT NULL
);
```

Migrations become an ordered list of `{version, name, fn}` applied in order,
each recorded on success, each skipped once recorded.

### 2.1 The transition is safe because the existing steps are idempotent

An existing database has no rows in `schema_migrations`, so on its next open
every migration is "unapplied" and runs — which is exactly what happens today,
and harmless precisely because each step is already idempotent. From the second
open onward they are skipped. No backfill of version rows is needed, and none is
attempted: pretending a migration was applied because a column happens to exist
is how a half-migrated database gets marked current.

### 2.2 Views and backfills stay outside the version gate

`ensureCatalogEntryView` and `ensureFactScoredView` **drop and recreate** their
views on every open, deliberately, so the view definition always matches the Go
source that declares it. Versioning them would mean a change to a view body did
nothing until someone remembered to bump a number — trading a cheap, always-true
property for a manual step. Views are recreated unconditionally, after the
versioned migrations.

`backfillCurrentFacts` is likewise a sync, not a schema change, and is already
guarded by its own emptiness check.

The rule: **a migration changes the schema's shape; a view or a sync restates it
from the current source.** Only the first is versioned.

## 3. Part two: memberships as the sole collection source

### 3.1 The columns and the table already disagree

`catalog_entries.collection_id` / `item_index` are written once, at insert. Since
then three paths have made them wrong:

- **Dedup.** `ingestObserveRecord` reuses an existing record entry and adds a
  membership. A record observed by three snapshots has `collection_id` naming
  only the first.
- **Removal.** `RemoveCollectionMembership` and `DeleteEntry` drop memberships
  without touching the column.
- **Pruning** (added 2026-07-27) deletes a snapshot's memberships and the
  snapshot row; any surviving shared record keeps a `collection_id` pointing at a
  collection that no longer exists.

So this is not a tidy-up. The column is a second answer to a question the
membership table already answers correctly, and it is already the wrong one.

### 3.2 What replaces them

`CatalogEntry.CollectionID` and `ItemIndex` stay on the struct — the public shape
does not change — but their meaning becomes precise:

- On **write**, `AddEntry` keeps reading `CollectionID` as "record a membership
  in this collection", which is what it already does.
- On a **collection-scoped read** (`GetCollectionItems`, `SnapshotRecordEntries`,
  `QueryEntries` with a `CollectionID` filter), they carry *that* collection and
  the item's index within it. This is the only reading that was ever
  well-defined for a shared item.
- On an **unscoped read**, they are populated only when the item belongs to
  exactly one collection, and are nil otherwise.

That last rule is the one that needs stating. An item in three snapshots has no
single collection ID; returning one is what the legacy column did, and it picked
the oldest by accident of insert order. Nil is the truthful answer, and
`pudl list` renders the membership line only when there is an unambiguous one.

Implementation: two correlated subqueries in the canonical select list, guarded
by `HAVING COUNT(*) = 1`. `collection_memberships` is indexed on `item_id`, so
each is an index lookup.

### 3.3 Dropping the columns

`ALTER TABLE catalog_entries DROP COLUMN` for both, preceded by dropping
`idx_collection_id` and `idx_item_index`, which reference them (SQLite refuses
otherwise). This runs as a versioned migration, *after* the membership backfill
that reads those very columns — so the backfill has to become conditional on
them existing, which is what makes it correct to run in either order on any
database.

`catalog_entry_edb` exposes `collection_id` to Datalog and must keep doing so:
the view takes the same derived expression, so a rule joining on `collection_id`
sees the unambiguous membership rather than a stale insert-time value.

### 3.4 While we are here: the rest of the row mapping

Recommendation 4's first slice centralized catalog row mapping into
`entryColumns` + `scanEntry`, but four read sites still hand-copy the column list
and its Scan (`GetCollectionByID`, `GetLatestObserve`, and both manifest reads in
`catalog_manifest.go`). Adding a derived column would silently skip them, so they
move onto the shared mapping as part of this change — otherwise this design
creates exactly the drift the first slice removed.

## 4. Blast radius

| Surface | Change |
|---|---|
| `internal/database` | `schema_migrations`; migration list; derived membership columns; drop two columns; remaining read sites centralized |
| `catalog_entry_edb` | `collection_id` becomes the derived expression |
| `cmd/list.go` | renders the membership line only when unambiguous (no code change needed — it already nil-checks) |
| `pkg/` | none |
| `CatalogEntry` | no field changes; `CollectionID`/`ItemIndex` semantics documented |

## 5. Tests

| Case | Assertion |
|---|---|
| Fresh database | every migration recorded once |
| Second open | no migration re-runs |
| Pre-existing database (no version rows) | all migrations run, then are recorded |
| Migration failure | not recorded; retried on the next open |
| Views recreated every open | a view is present after a manual DROP |
| Item in one collection, unscoped read | `CollectionID` and `ItemIndex` populated |
| Item in two collections, unscoped read | both nil |
| Collection-scoped read | carries *that* collection and index |
| Membership removed | the entry no longer claims the collection |
| Legacy database with populated columns | backfilled into memberships, then dropped |
| `catalog_entry_edb.collection_id` | matches the unambiguous membership |

## 6. Adversarial review

**A1 — "Running every migration on a legacy database because the version table
is empty defeats the point."** *Accepted for exactly one open.* The alternative
is inferring applied-ness from the schema (does this column exist?), which marks
a half-migrated database as current — the failure the version table exists to
prevent. One extra pass of idempotent statements, once per database, is the
cheaper mistake.

**A2 — "Nil `CollectionID` for a shared item breaks `pudl list`."** *Checked.*
`cmd/list.go:319` already guards on `entry.CollectionID != nil`, so a shared item
simply omits the membership line. It previously printed one collection out of
several with no indication that was happening.

**A3 — "Two correlated subqueries per row will be slow on a large `list`."**
*Accepted with a bound.* `idx_collection_memberships_item` makes each an index
lookup on a table with one row per membership. If it ever shows up, the fix is a
`LEFT JOIN` against a grouped subquery — one pass — which is a rewrite of the
select list and nothing else.

**A4 — "Dropping columns is irreversible; a rollback to older pudl breaks."**
*True, and stated rather than mitigated.* An older binary would `SELECT
collection_id FROM catalog_entries` and fail. This is a one-way migration, which
is what retiring a column means; the alternative is keeping a wrong second answer
forever. The membership data is strictly richer, so nothing is lost but the
ability to run an older binary against a migrated catalog.

**A5 — "Excluding views from versioning is inconsistent."** *Deliberate, and the
rule is stated in §2.2.* A view is a restatement of the current source, not an
increment on past state. Versioning it would make a view-body change a no-op
until someone bumped a number — a foot-gun with no upside.

**A6 — "Centralizing the four remaining read sites is scope creep."** *Rejected.*
Adding a derived column to `entryColumns` without moving them means those four
reads keep selecting a column that no longer exists. They are not adjacent work;
they are the same work.

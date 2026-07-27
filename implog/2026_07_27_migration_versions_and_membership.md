# A migration version table, and memberships as the sole collection source

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-migration-versions-and-membership.md`
**Closes:** Recommendation 4's two outstanding items

## What was wrong

Every `NewCatalogDB` re-ran every schema step, correctness resting on each author
remembering to write its own idempotency guard. There was no record of what a
given database had actually had applied to it, so a half-migrated catalog was
indistinguishable from a current one.

Separately, `catalog_entries.collection_id` / `item_index` were a second answer
to a question `collection_memberships` already answers — and already the wrong
one. Written once at insert, three paths had since made them stale: dedup reuses
a record entry and adds a membership (a record in three snapshots named only the
first), removal drops memberships without touching the column, and pruning
(added the same day) deletes a snapshot while any surviving shared record keeps a
`collection_id` pointing at a collection that no longer exists.

## What changed

**`schema_migrations`.** Migrations are an ordered list applied in order and
recorded on success; a failure leaves its version unrecorded and stops the run,
so it is retried next open and nothing after it may assume a shape that was never
built. Applied-ness is never inferred from the schema — that is exactly how a
half-migrated database gets marked current. An existing catalog has no version
rows, so on its next open everything runs once more (harmless, since each step is
still idempotent) and is then recorded.

**Views and syncs stay outside the version gate**, stated as a rule: a migration
changes the schema's shape; a view or a sync restates it from the current source.
Versioning `catalog_entry_edb` would make a change to its body a no-op until
someone bumped a number.

**The legacy collection columns are dropped.** `CatalogEntry.CollectionID` /
`ItemIndex` keep their place on the struct with sharpened meaning:

- on write, `CollectionID` means "record a membership in this collection" — what
  it already did;
- on a collection-scoped read, they carry *that* collection and the index within
  it, the only well-defined answer for a shared item;
- on an unscoped read, they are populated only when the item belongs to exactly
  one collection, and nil otherwise.

`catalog_entry_edb.collection_id` uses the same derived expression, so a Datalog
rule joining on it sees the unambiguous membership rather than a stale
insert-time value. `cmd/list.go` needed no change — it already nil-checked, and
now omits the membership line for a shared item instead of naming one of several.

**Row mapping finished.** Recommendation 4's first slice centralized reads into
`entryColumns` + `scanEntry`, but seven sites still hand-copied the list and its
Scan (`FindByProquint`, `QueryEntries`, `GetCollectionItems`,
`GetCollectionByID`, `FindByResourceID`, `GetLatestObserve`, and both manifest
reads). Adding a derived column would have silently skipped them, so they moved
onto the shared mapping. `entryColumns` is now `entrySelect(alias)` /
`entrySelectVia(entryAlias, membershipAlias)`.

## Behaviour change found while implementing

`backfillDefaults` (content_hash = id, version = 1) was doing double duty: a
one-time migration *and* an ongoing repair, because it re-ran on every open. A
row written by `AddEntry` without those fields only became complete when the next
process opened the catalog. Under recorded migrations that repair happens once,
so the defaults moved to where the row is created — `addEntryIn`, beside the
existing status default. Same end state, arriving immediately instead of after a
restart. Pinned by `TestAddEntry_DefaultsContentHashAndVersionAtInsert`.

## Public API

- `internal/database`: `AppliedMigrationVersions() ([]int, error)`; new
  `schema_migrations` table; `catalog_entries.collection_id` and `item_index`
  dropped. `CatalogEntry` fields unchanged; `CollectionID`/`ItemIndex` semantics
  documented above.
- Unexported: `migration`, `migrations`, `runMigrations`, `entrySelect`,
  `entrySelectVia`, `retireLegacyCollectionColumns`, `migrateCatalogEntries`.

## Tests

- `internal/database/migrations_test.go` — a fresh database records every
  version; a second open re-runs nothing (asserted on `applied_at` stability); an
  unrecorded migration is applied (the transition case); a failure is not
  recorded and stops everything after it; views are rebuilt even when every
  migration is already recorded; the legacy columns are gone; a legacy database
  is backfilled into memberships *then* has its columns dropped.
- `internal/database/collection_membership_semantics_test.go` — sole membership
  reported; shared item reports none; collection-scoped read carries that
  collection; removing a membership stops the claim; a shared item down to one
  membership becomes unambiguous again; `QueryEntries` collection scoping; the
  EDB view matches; insert-time defaults.

## Not done here

Nothing outstanding from Recommendation 4. Note this is a **one-way** migration:
an older pudl binary would select a column that no longer exists. That is what
retiring a column means, and the membership data is strictly richer.

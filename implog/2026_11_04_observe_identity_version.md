# pudl-xr6: an entry that gains an identity becomes its resource's next version

**Date**: 2026-11-04
**Type**: Bug fix (follow-up to pudl-42w, `implog/2026_11_04_observe_versions.md`)

## Problem

In `observe.PreparedObservation.Commit`, a re-observed record whose content is
already in the catalog is deduplicated to the existing entry. When that entry
was stored without an identity and the record now has one, Commit calls
`UpdateEntryIdentity`, which moved the entry to the identified `resource_id`
but kept its version. The entry had been versioned alone (its content-hash
resource, version 1), so the identified resource could end up holding two
entries at the same version. pudl-42w fixed this for new entries (latest + 1)
but not for this move.

## Fix

`database.updateEntryIdentityIn` (behind `CatalogDB.UpdateEntryIdentity` and
`CatalogTx.UpdateEntryIdentity`) now sets the version in the same `UPDATE`:
when the resource changes, the entry becomes the new resource's
`MAX(version) + 1` (1 if the resource has no versions); when the resource does
not change, the version is kept. The move and its version cannot diverge, and
inside Commit's transaction a later new record of the same resource in that
observation reads the updated latest version. Commit's only change is a
comment pointing at this.

## API changes

None in signatures. `UpdateEntryIdentity` (internal) now also assigns the
version as described. Its only caller is observe Commit.

## Tests

- `internal/observe/version_test.go`
  `TestIngestObserve_IdentifiedRecordIsNextVersionOfItsResource`: observes
  thing-1 "a" with its schema (version 1), then "b" without schemas (stored
  without an identity), then "b" again with the schema. "b" must be version 2
  of thing-1. Without the fix it stays at version 1.
- `internal/database/catalog_identity_test.go`
  `TestUpdateEntryIdentityVersionsTheMovedEntry`: a moved entry takes
  latest + 1, an entry re-identified within its own resource keeps its version.

Catalogs that already hold such duplicates can be renumbered with the existing
opt-in doctor repair `versions` (pudl-42w).

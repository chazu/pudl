# 2026-06-29 — Remove the legacy `method` column; confirm artifact resource_type is live (vestige residual)

Closes the §1.3 / §5.2 "residual" items from `docs/vestige-sweep.md`.

## 1. `method` column — REMOVED

The `catalog_entries.method` column was the last load-bearing-for-nothing remnant of
the removed definition→method→artifact execution model (the executor moved to mu; the
World A drift checker that keyed on definition+method was deleted in Cluster A).

**Verified dead before removing:** `grep` found zero non-nil writes to `CatalogEntry.Method`
anywhere — it was only ever scanned/inserted as NULL. `entry_type='artifact'` is likewise
never written (live values: `import` default, `observe`, `manifest`, `manifest-action`).

**Removed:**
- `database.CatalogEntry.Method` struct field.
- All SQL touching it: INSERT column list + placeholder + arg, UPDATE SET + arg, and every
  SELECT column list + `Scan` target across `catalog.go`, `catalog_manifest.go`,
  `catalog_observe.go`, `catalog_identity.go`.
- `method` from the `catalog_entry_edb` view (`catalog_entry_view.go`) — this drops it from
  the Datalog `catalog_entry` relation surface (it bound by column name; always NULL, so no
  real rule could use it). `docs/datalog.md` relation table updated to match.
- The `lister.CatalogEntry.Method` mirror field + its mapping (set-only, never read).
- Migration: removed the `method` ADD COLUMN and the `idx_definition_method` index.

**Existing databases:** new `dropLegacyMethodColumn()` (called from `ensureArtifactColumns`)
idempotently drops the column on reopen. It drops the `catalog_entry_edb` view first
(SQLite blocks `DROP COLUMN` while a view references the column — `ensureCatalogEntryView`
recreates the view later in the open sequence), then the index, then the column.
Regression test: `TestDropLegacyMethodColumn` reproduces the view-references-column state and
asserts the drop + idempotency.

`Definition` was **kept** — it is now live, repurposed for run verdicts
(`definition='//models/<name>'`); its comment was updated to say so.

## 2. artifact `resource_type` axis — CHECKED, KEEP (false positive)

`internal/importer/bootstrap/pudl/artifact/artifact.cue` (`#ImageRef`, `#ArtifactRef`) is a
legitimate standalone **data** schema for container-image refs and content-addressed
artifacts — `schema_type:"base"` with identity/tracked fields, registered in `catalog.cue`.
The `resource_type` axis is *data classification* and is distinct from (only name-colliding
with) the dead `entry_type='artifact'`. Nothing ties it to the removed execution model. No
change — recorded as a false positive in §5.2, like §1.4/1.5.

## Public API delta

- `database.CatalogEntry` and `lister.CatalogEntry` no longer have a `Method` field.
- The Datalog `catalog_entry` relation no longer exposes a `method` column.

## Verification

`CGO_ENABLED=0 go build ./...` clean; `CGO_ENABLED=0 go test ./...` all green (incl. the new
`TestDropLegacyMethodColumn`).

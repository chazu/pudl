# Sensitive-data scrub — pudl-m10

`pudl doctor` now diagnoses plaintext stored before a schema declared the path
in `_pudl.sensitive_fields`, and the opt-in repair `sensitive-scrub` redacts
it. The design is `docs/design/2026-11-01-sensitive-data-scrub.md`; its body
was revised first to the 2026-11-01 decisions (re-derive IDs, leave fact
history unchanged).

## Behavior

- **Diagnosis** (`pudl doctor`, read-only). The repair streams the catalog
  (`EachEntryContext`, honoring `--entry`/`--schema`/`--origin`) and checks
  each payload by path under `redact.Registry.For(intended, assigned)`, the set
  import uses. It is gated on `Registry.Any()`, so a catalog with no sensitive
  declarations reads nothing. Collection parents of flagged items are
  re-encoded with the import framing. Collected values of 8 bytes or more are
  searched for (in memory only) in every `.meta`, in legacy `pudl/mu`
  manifest/drift entries and observe snapshot files, and in stored run,
  run-set and approval JSON. Diagnosis builds the same plan as apply, so the
  preview is exact. Non-JSON or compressed payloads are reported with the fix
  "delete and re-import as JSON".
- **Opt-in.** `Repair.OptIn`: plain `--fix` reports `skipped` ("opt-in: name
  it with --only sensitive-scrub"); `--fix --only sensitive-scrub` applies,
  `--dry-run` previews.
- **Apply.** Under `artifacts.WithLock`: finish an earlier interrupted scrub
  (unlink superseded files no entry references, compact if pending); publish
  each rewritten file at a new path through `artifacts.Journal`; then one
  transaction on a `secure_delete` connection re-keys entries, re-projects
  current facts, rewrites matched report JSON, rebuilds the facts FTS index and
  writes the audit rows; after commit, unlink superseded files
  (`RemoveCommittedOrphan`) and compact (`wal_checkpoint(TRUNCATE)`, `VACUUM`,
  checkpoint, `compacted_at`). A busy checkpoint reports "compaction pending"
  and fails `--fix` until a rerun finishes it.
- **IDs.** Content-derived entries (`id = content_hash`) get the hash import
  would compute (bytes for documents and collections, canonical JSON for items
  and observe records). Snapshots keep their pre-allocated IDs. Identity-less
  `resource_id`s still equal to `H(namespace, old hash)` are recomputed;
  resources with identity values keep theirs, and the scrub refuses an entry
  whose identity values would change.
- **Fact history** is not touched. Current projected facts of re-keyed entries
  are re-projected as corrections under the new ID.
- **Messaging.** Every finding or apply says that fact history and exported
  bundles keep the plaintext, old IDs remain in redirects, and the remedy is
  to rotate the credential. The secret-scan fix text now names the scrub.
  Values never appear in output, errors or audit rows.

## ID-bearing references the scrub rewrites

`catalog_entries.id`, `item_id`, `origin` (import items' `<collection>_item_N`),
`content_hash`, `stored_path`, `metadata_path`, `size_bytes`, identity-less
`resource_id`; `collection_memberships.collection_id` and `item_id`;
`item_schemas.item_id`; `policy_state.entry_id`; `projection_state.entry_id`
(and `resource_id`) and current projected facts' `entry_id` arg and
provenance, via re-projection; `.meta` `id`, `content_hash`, `resource_id`,
`size_bytes`. Unchanged by design: snapshot-ID tables (`observe_snapshots`,
`snapshot_pins`, `report_snapshot_refs`, tombstones, reuse blocks: snapshot IDs
are never content-derived), report JSON IDs and fact history (resolved through
redirects), policy facts' provenance (no values; reconcile compares args), and
exported bundles.

## Collisions, redirects, crash safety

- **Merge.** When the new ID already names an entry (re-imported after the
  declaration, or two entries that differed only in the secret), the survivor
  keeps its files; the scrubbed row's references move to it (`UPDATE OR
  IGNORE`, then delete the rest), the row is deleted, and its files are
  unlinked after commit. Planned at diagnosis and re-checked in the
  transaction.
- **Redirects.** `sensitive_id_redirects(old_id, new_id, scrub_id)`;
  `CatalogDB.ResolveEntryRedirect` follows chains and accepts proquints;
  `lister.FindEntry` (behind `pudl show`, `pudl delete`) falls back to it.
- **Crash safety.** New file names follow new content (old ID token replaced,
  or `.r<hash8>`), so a crash before commit leaves only unreferenced new files
  that a rerun reuses byte for byte; the catalog and old files are untouched.
  A crash after commit leaves the superseded files listed in
  `sensitive_redactions.old_files`; the next diagnosis reports them and the
  next apply unlinks them and finishes compaction.
- **Dedup fix** from the first draft (`findByContentHashIn ... OR id = ?`) is
  not needed: re-derivation keeps `id = content_hash`, and a later re-import
  of the plaintext hashes to an old ID that no longer names a row.

## Public API

- `internal/scrub`: `Scrubber{DB, Inferrer, Projections, Scope}`,
  `(*Scrubber).Plan(ctx) (*Plan, error)`,
  `(*Scrubber).Apply(ctx, dryRun) (*Outcome, error)`; `Plan.Subjects()`,
  `Plan.Pending()`, `Plan.ShortValuesSkipped()`, `Plan.Unsupported`,
  `Plan.Errors`, `Plan.Leftovers`, `Plan.Uncompacted`; `Subject`, `Scope`,
  `Outcome`.
- `internal/database`: migration 27 (`sensitive_scrubs`,
  `sensitive_redactions`, `sensitive_id_redirects`);
  `CatalogTx.RekeyScrubbedEntry(ScrubbedEntry) (merged bool, error)`,
  `CatalogTx.RecordSensitiveScrub`, `CatalogTx.RewriteReportBlob`,
  `CatalogTx.RebuildFactsFTS`, `CatalogDB.WithSecureCatalogTxContext`,
  `CatalogDB.CompactScrubbed`, `ErrCompactionBusy`,
  `CatalogDB.ResolveEntryRedirect`, `CatalogDB.PendingScrubs`,
  `CatalogDB.IsArtifactReferenced`, `CatalogDB.EachReportBlob`,
  `CatalogDB.ItemCollections`, `ReportBlob`, `SensitiveRedaction`.
- `internal/importer`: `EncodeRecord(collection, index, record)` (shared with
  the transform), `WithheldValidationMessage`.
- `internal/projection`: `Correct(tx, prepared)`.
- `internal/repair`: `Repair.OptIn`; the `sensitive-scrub` repair (last in
  the registry).

## Tests

`internal/scrub`: detection of an entry imported before the declaration;
dry run writes nothing (catalog tables and data files compared); payload,
`.meta`, collection items and parent, snapshot file and report JSON redacted;
IDs re-derived to the import hash and no catalog column (memberships,
projection and policy state, `item_id`) holds an old ID; old IDs redirect;
current facts re-projected without plaintext; every pre-existing `facts` row
unchanged; re-imports deduplicate; payload hashes verify; collision with an
existing entry and between two scrubbed entries merges; a crash after
publication and before commit leaves the catalog unchanged and the rerun
reuses the files; a crash after commit is reported and finished by the next
run; a second run is clean and writes nothing. `cmd`: opt-in `skipped` under
plain `--fix`, `needed`/`applied`/`ok` statuses, rotation messaging, `pudl
show <old proquint>` resolves, and no doctor, dry-run, apply or show output
contains the value.

## Size

Net delta against `sre-focus` (after merging it): production **+1695 / −16
(net +1679)**, tests **+604 (net +604)**. Without comments and blank lines the
new production files hold 1363 lines. This is above the design's 650 (±150)
estimate and the ~1,000 threshold. Re-derivation is the reason: re-keying with
merge and the reference list (~155), redirects and lookup fallback (~60), the
crash-safe publication, audit and unlink path (~240 in `apply.go`), plan-time
merge detection, and per-kind re-encoding so new IDs equal import's
(collection framing, item vs observe storage, renaming; ~205 in `rewrite.go`).
Value search over four artifact kinds (~250) and compaction (~50) were in the
original estimate but undercounted.

## Not safe / not done

- Fact history, exported bundles and backups keep plaintext and old IDs; old
  IDs also stay in redirects, report JSON, policy facts' provenance and the
  file names/origins of unchanged collection items, so a low-entropy secret
  remains brute-forceable from them. Rotation is the remedy, as doctor says.
- Non-JSON and compressed payloads are reported, not scrubbed.
- Plans hold redacted payloads in memory (bounded by import limits).
- Diagnosis reads each entry's `.meta` when any schema declares sensitive
  fields, to learn the intended schema.
- `pudl show` does not print a "redacted after import" line (dropped from the
  design in favour of redirects).

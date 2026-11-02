# Sensitive-data scrub: redact plaintext stored before a field became sensitive

**Date:** 2026-11-01 (body revised to the decisions on 2026-11-01)

**Status:** Accepted with changes on 2026-11-01; see
[Decisions](#decisions-2026-11-01). Tracked by beads `pudl-m10`. Follows
[GCP cataloging UX](2026-10-08-gcp-cataloging-ux.md) §6.2 ("Retroactive").
The first draft recommended keeping entry IDs and rewriting fact history under
an exception to append-only transaction time. The user overrode both: IDs are
**re-derived**, and fact history is **left unchanged**. The body below has been
revised to match; the decisions section still governs.

## Decision

Add one opt-in `pudl doctor` repair, `sensitive-scrub`.

- **Diagnosis** runs with every `pudl doctor`. It reports stored plaintext at
  paths that a schema now declares in `_pudl.sensitive_fields`, and the
  unstructured artifacts that quote it. It names entry IDs, report IDs, file
  paths and field paths, and never prints values.
- **Apply** runs only when named:
  `pudl doctor --fix --only sensitive-scrub`. Add `--dry-run` to preview. It
  rewrites each affected artifact so that it holds what a fresh import would
  store today: `"[REDACTED]"` at each sensitive path. It then compacts SQLite
  so that old values do not survive in the WAL or in free pages.
- **Entry IDs are re-derived** from the redacted bytes. Every column that
  stores the old ID is rewritten in the same transaction; an ID that already
  exists is merged into. Old-to-new mappings are recorded, and entry lookups
  follow them, so reports and fact history that cite old IDs still resolve.
- **Fact history is not touched.** The append-only invariant stands. Current
  projected facts are corrected by re-projection. Doctor says that history
  and exported bundles keep the plaintext, and that the remedy is to rotate
  the credential.

## Problem

Redaction (`internal/redact`) runs at import and observe time only.
Projection redacts in memory (`internal/projection/sync.go`), and because the
sensitive set is part of a spec's fingerprint, the `projections` repair
already re-projects current facts once a field becomes sensitive. Everything
written before the declaration still holds plaintext:

- raw payloads, collection files and item files;
- `.meta` files;
- history rows in `facts` (the projections repair closes rows; it does not
  remove them);
- report JSON;
- legacy mu-era entries;
- SQLite pages that once held deleted `current_facts` and FTS rows.

The doctor secret scan (`implog/2026_11_01_doctor_secret_scan.md`) finds
likely secrets *outside* schema coverage. This check covers the complement:
values *inside* declared coverage that were never redacted.

## 1. Detection

Detection uses one of two exact methods, depending on the artifact.

- **Path-based**, for artifacts with a schema. The redaction set is the same
  one import uses: `redact.Registry.For(intended, assigned)`, a union along
  `base_schema`. The intended schema is read from the entry's
  `.meta` (`schema_info.intended_schema`). A value found at a sensitive path
  (`fieldpath.LookupAll`) that is not `"[REDACTED]"` is a finding. A broken
  sensitive declaration means the entry cannot be evaluated; it is reported
  as an error and is never reported as clean.
- **Value-based**, for unstructured artifacts. The scrub collects the
  plaintext strings from the path-based findings in this run, holds them in
  memory only, and searches the remaining artifacts for them as substrings of
  decoded JSON strings. Only values of at least 8 bytes are searched, because
  shorter ones (`true`, `admin`) would redact unrelated text. The scrub counts
  the shorter values it skipped and reports that count.

| Artifact | How plaintext got there | Detect | Scrub |
|---|---|---|---|
| Document payload (JSON) | Imported before the declaration | Path, under the entry's schemas | Re-encode the redacted record exactly as the import transform does (`importer.EncodeRecord`) |
| Collection item / observe record | Same | Path, under the item's schemas | Import items: `json.Indent(json.Marshal(redacted))`, as the collection stream stores them. Observe records: `json.Marshal(redacted)`, as observe stores them |
| Collection parent file | Holds every item record | Flagged when any member item is flagged | Re-encode each record with the import framing (NDJSON or array), redacted under the schemas of the member item at that `collection_memberships.item_index` |
| Non-JSON or compressed payload | Imported before the declaration; import now fails closed for these | Path, after `DecodeStoredFile` | **Not scrubbed.** Reported with the fix "delete the entry and re-import as JSON". Import refuses to store these formats unredacted, so the scrub does not convert them either. |
| `.meta` file | `validation_errors` and trace reasons quote CUE values | Rewritten with its entry, plus a value search of every `.meta` | Replace each `validation_errors` message with import's withheld message, update `id`, `content_hash`, `resource_id` and `size_bytes`, then run the value search |
| Current projected facts | Projected before the declaration, or citing the old entry ID | The entry is flagged and is its resource's projected entry (`projection_state.entry_id`) | Re-project from the redacted record under the new ID, as a correction (§3) |
| Fact history | Projected before the declaration | Not scanned | **Left unchanged** (§3). Doctor states this |
| `current_facts_fts` | FTS5 segments keep tokens after rows are deleted | Implied by any re-projection | `INSERT INTO current_facts_fts(current_facts_fts) VALUES('rebuild')` |
| `run_reports`, `run_set_reports` JSON | Check witnesses (fact args), and drift `expected`, `observed` and `previous.value` | Value search | Rewrite matching strings with an `UPDATE` (not `SaveRunReport`, which resets pins) |
| Legacy `run_approvals` and `run_set_approvals` `request_json` | Convergence-era plans and diffs | Value search | Same as reports |
| Legacy `pudl/mu.#DriftObservation`, `#Manifest`, `#ManifestAction` and `#PlanOutput` entries | `pudl-bq5` and `pudl-8b7` stored them verbatim | Value search (manifest outputs have no schema, so the path method cannot apply) | Rewrite the file like a payload; re-derive the ID |
| Observe snapshot files (`pudl/mu.#ObserveSnapshot`) | Summary and per-record `errors` strings | Value search | Rewrite the file like a payload; the snapshot ID is kept (§2) |
| SQLite WAL and free pages | Old page images, plus deleted `current_facts`, FTS and report rows | Not scanned. A scrub records whether compaction finished (§4), and doctor reports a scrub whose compaction is pending | §4 |
| Superseded files from an interrupted scrub | Crash after commit, before unlink | The audit table lists the old paths; any that still exist and no entry references are findings | Unlink them |
| `data/tmp` and `.staging-*` leftovers | A SIGKILLed import (§6.2) | Not reported by this repair | **Not touched.** A concurrent import's temp files live there before it takes the lock |
| Exported bundles and backups | Outside the workspace | Cannot detect | Out of scope (§6) |

**Identity is unaffected, and the scrub verifies it.** `redact` already makes
a sensitive path that overlaps the schema's own identity fields a load error.
The scrub also extracts each entry's identity values from the original and
from the redacted record, and refuses the entry if they differ. Resources with
identity values therefore keep their `resource_id`, versions and projection
source. Identity-less resources are covered in §2.

**Values are never printed.** Findings carry the subject kind and ID, the
schema, and the path. Collected values live only in process memory. Dry-run,
JSON output, error messages and the audit tables never contain them.

**Placement.** The repair's `Diagnose` streams the catalog itself
(`EachEntryContext`, honoring `--entry`, `--schema` and `--origin`), like the
doctor secret scan. It is gated on `redact.Registry.Any()`: when no loaded
schema declares a sensitive field, it reads nothing. Otherwise it reads the
`.meta` and payload of each entry whose schemas declare sensitive paths.
Diagnosis and `--dry-run` build the same plan as apply and write nothing, so
the preview is exact.

## 2. Content addressing

A content-derived entry ID is the SHA-256 that import computes. A scrub
changes the content, so it **re-derives the ID** with the same rule:

| Entry kind | New ID |
|---|---|
| Document, collection parent, legacy mu entry | SHA-256 of the new stored bytes |
| Import collection item, observe record | SHA-256 of `json.Marshal` of the redacted record (canonical JSON) |
| Observe snapshot (`id != content_hash`: keyed on its pre-allocated `snapshot_id`) | **Kept.** Only `content_hash`, paths and size change |

An entry is content-derived when its `id` equals its `content_hash`. The
scrub refuses to re-key an entry that has an `observe_snapshots` row.

**`resource_id`.** For resources with identity values it is unchanged (§1).
For identity-less resources, import sets it to
`H(identity namespace, content hash)`, which is itself an oracle. When it still
equals that value, the scrub recomputes it from the new ID, as a fresh import
would. Any other value (for example, set by an older pudl) is kept. This
refines decision 1's "`resource_id` is unchanged": its reason, that identity
fields cannot be sensitive, covers only resources with identity values.
Identity-less resources are never projected (`projection_state` records them
as skipped), so no current fact changes source.

**References.** These are every table and column that stores an entry ID, and
how the scrub treats each one in the rewrite transaction:

| Table.column | Treatment |
|---|---|
| `catalog_entries.id` | Rewritten to the new ID, or the row is merged into an existing entry |
| `catalog_entries.content_hash`, `stored_path`, `metadata_path`, `size_bytes` | Set from the redacted files |
| `catalog_entries.item_id` | Rewritten where it equals the old ID (import items) |
| `catalog_entries.origin` | Import items carry `<collection id>_item_<n>`; rewritten when their collection is re-keyed |
| `catalog_entries.resource_id` | Recomputed for identity-less resources (above); otherwise unchanged |
| `catalog_entries.version` | Unchanged. A merge removes one row of a version chain, which can leave a gap; versions are ordered, not dense |
| `collection_memberships.collection_id`, `item_id` | Rewritten; on a merge, a membership the surviving entry already has is kept and the duplicate dropped |
| `item_schemas.item_id` (legacy) | Rewritten, with the same merge rule |
| `projection_state.entry_id` (and `resource_id` for identity-less resources) | Re-projected under the new ID (§3); repointed when the schema no longer projects |
| `current_facts` args `entry_id` and `provenance.entry_id` | Re-projected (§3) |
| `policy_state.entry_id` (policy results, `pudl-xf5.8.3`) | Repointed. Policy facts carry no record value and no entry ID in their args, so they are not re-evaluated; their `provenance.entry_id` keeps the old ID, which redirects |
| `facts` (history) args and `provenance` | **Unchanged** (§3) |
| `observe_snapshots.snapshot_id`, `snapshot_pins.snapshot_id`, `report_snapshot_refs.snapshot_id`, `observe_snapshot_tombstones`, `snapshot_reuse_blocks` | Snapshot IDs are pre-allocated, never content-derived, so they never change |
| `run_reports`, `run_set_reports`, `run_approvals`, `run_set_approvals` JSON (`evidence_ids`, `entry_id`) | Not rewritten for IDs. They are records of what a run saw; the redirects resolve them |
| `runs` | Holds run IDs only |
| `.meta` `id`, `resource_tracking.content_hash`, `resource_id` | Rewritten with the entry |
| Exported bundles | Outside the catalog; keep old IDs and plaintext (§6) |

**Collisions merge.** The new ID can already exist: the source was re-imported
after the declaration, or two scrubbed entries differed only in the secret.
The existing entry has the same redacted content, so it survives. The scrubbed
row is deleted after its references move to the survivor
(`UPDATE OR IGNORE`, then `DELETE` of what is left), its projection is
re-pointed to the survivor, and its files become unreferenced and are unlinked
after commit. The audit row records the merge.

**Redirects.** `sensitive_id_redirects(old_id, new_id, scrub_id)` records each
re-keyed or merged entry. `lister.FindEntry` (behind `pudl show`, `pudl
delete` and other entry lookups by ID or proquint) falls back to it when an ID
is not found, following chains across scrubs. So `pudl show <old proquint>`,
report evidence IDs and the `entry_id` of historical facts still resolve. A
current entry with that ID always wins over a redirect.

**What re-derivation does not hide.** The old IDs remain in the redirect
table, in fact history (`entry_id` args and provenance), in stored report JSON,
and in the file names and origins of unchanged collection items. Each old ID is
a hash of content that included the secret, so a low-entropy secret (a PIN,
a dictionary password) can still be brute-forced from it. The live catalog
columns no longer carry it. Rotating the credential is the remedy, and the
scrub's output says so every time it redacts anything.

**Dedup needs no change.** The first draft added `OR id = ?` to
`findByContentHashIn`, because keeping IDs broke `id = content_hash`. Under
re-derivation every content-derived entry keeps `id = content_hash`. If the
field is later un-declared and the plaintext re-imported, its hash equals the
old ID, which no longer names a row, so import creates a new entry without a
primary-key collision. Re-importing the source while the field is sensitive
hashes to the new ID and deduplicates.

**The encoder is shared.** The import transform's per-record encoding and
collection framing are one function, `importer.EncodeRecord`, which the scrub
also calls. "What a fresh import would store" holds by construction, and a
re-import deduplicates byte for byte.

## 3. Fact history

History rows in `facts` keep their plaintext args and old entry IDs. The
append-only invariant in `docs/facts.md` and `docs/library-api.md` is
unchanged: no operation rewrites what the store believed at an earlier moment,
and `pudl facts list --as-of-tx` keeps returning what was projected.

**Current facts are corrected by re-projection.** In the rewrite transaction,
for each scrubbed entry that is its resource's projected entry, the scrub
computes the projection from the redacted record under the new ID and applies
it as a correction (`projection.Correct`, the mode the `projections` repair
uses). The current rows that cite the old ID, or hold plaintext because the
`projections` repair has not run since the declaration, are closed; rows with
the new ID and `"[REDACTED]"` are added; `projection_state` points at the new
ID. Old current rows leave `current_facts` and the FTS index, and §4 removes
their bytes from free pages. They stay in `facts` as history.

Facts from `pudl facts add` have no schema and are out of scope. The secret
scan covers them.

## 4. SQLite

The rewrite transaction runs on one pinned connection with
`PRAGMA secure_delete=ON`, so the bytes freed by its own updates are zeroed.
It also rebuilds the FTS index
(`INSERT INTO current_facts_fts(current_facts_fts) VALUES('rebuild')`), so
tokens of deleted rows leave the index in the same commit. After the commit,
still holding the artifact lock, the scrub compacts:

1. `PRAGMA wal_checkpoint(TRUNCATE)`, which copies the new pages over the old
   ones and truncates the WAL.
2. `VACUUM`. This is the only step that removes plaintext left in free pages
   by *earlier* deletes, such as the `current_facts` and FTS rows deleted when
   the projections repair re-projected. `VACUUM` makes a logical copy. The
   scrub has already committed by then, so the copy holds no scrubbed value
   outside fact history.
3. `PRAGMA wal_checkpoint(TRUNCATE)` again, because in WAL mode `VACUUM`
   writes through the WAL. Then set `compacted_at`.

**Cost.**

- `VACUUM` takes time proportional to the catalog's size. The global store is
  6 MB today, so this is well under a second.
- It needs roughly twice the database size in free disk while it runs.
- `secure_delete` adds write I/O only during the scrub transaction.

**Locking.**

- The artifact lock (`artifacts.WithLock`) excludes imports, other repairs
  and bundle export.
- A read-only `pudl` process holding a WAL snapshot can make the checkpoint
  report busy, or make `VACUUM` fail with `SQLITE_BUSY` after the connection's
  busy timeout. The scrub then returns `partial` with "compaction pending".
  Doctor keeps reporting it until a rerun completes it.

Compaction runs whenever a scrub commits; every scrub rewrites catalog rows.

## 5. Workflow and safety

**Commands.**

- `pudl doctor` diagnoses: status `needed`, with the subject list and the
  preview and apply commands from `repairArgv`.
- `pudl doctor --fix` skips the scrub. Its status is `skipped`, with the
  message "opt-in: name it with --only sensitive-scrub".
- `pudl doctor --fix --dry-run --only sensitive-scrub` previews: each subject,
  old and new ID, paths, files to unlink, and whether it will compact.
- `pudl doctor --fix --only sensitive-scrub` applies.

The scope flags `--entry`, `--schema` and `--origin` select payloads. Values
collected from the selected payloads are still searched for everywhere.

**Confirmation.** Naming the repair is the confirmation. There is no
interactive prompt, so agents and scripts work the same way as people. The
repair has `OptIn: true`. `Select` still returns it for diagnosis, and
`runDoctorRepairs` applies it only when it was named in `--only`.

Findings do not fail `pudl doctor`, which matches the other repairs' `needed`
status. A failed apply returns `partial` or `error`, which fails `--fix`, as
it does today.

**Audit record.** A migration adds three tables. They hold no values.

```sql
CREATE TABLE IF NOT EXISTS sensitive_scrubs (
  scrub_id     TEXT PRIMARY KEY,     -- random proquint
  scrubbed_at  TIMESTAMP NOT NULL,   -- written in the rewrite transaction
  compacted_at TIMESTAMP,            -- set after §4
  summary      TEXT NOT NULL         -- JSON counts per subject kind, plus short values skipped
);
CREATE TABLE IF NOT EXISTS sensitive_redactions (
  scrub_id     TEXT NOT NULL,
  subject_kind TEXT NOT NULL,        -- entry | metadata | run_report | run_set_report | run_approval | run_set_approval
  subject_id   TEXT NOT NULL,        -- the entry's ID before the scrub, or the report key
  new_id       TEXT,                 -- the re-derived ID, when it changed
  merged       INTEGER NOT NULL DEFAULT 0,
  paths        TEXT NOT NULL,        -- JSON: field paths, or "value-match"
  old_files    TEXT,                 -- JSON: superseded stored and metadata paths
  PRIMARY KEY (scrub_id, subject_kind, subject_id)
);
CREATE TABLE IF NOT EXISTS sensitive_id_redirects (
  old_id   TEXT PRIMARY KEY,
  new_id   TEXT NOT NULL,
  scrub_id TEXT NOT NULL
);
```

`CatalogEntry` gets no new column, so the SELECT and Scan lists do not change.

**Crash safety.** This reuses the import and delete patterns.

1. Take `artifacts.WithLock`. Finish any earlier scrub first: unlink its
   `old_files` that still exist and no entry references, then compact if
   `compacted_at` is null.
2. Plan the changes in memory: redacted bytes, new IDs, new paths.
3. Write each new file into `data/tmp`, and install it at a **new** path
   through `artifacts.Journal.Publish`. `Publish` never overwrites, so the old
   bytes stay in place until commit. A new name replaces the old ID (or its
   16-hex prefix) with the new one; a name with no ID token gets `.r<hash8>`
   before its extension, from the new bytes. Names are a function of the new
   content, so a rerun after a crash publishes to the same paths and reuses
   identical bytes.
4. In one catalog transaction, re-key or update the entries and every
   reference (§2), re-project (§3), rewrite report JSON, rebuild FTS, and
   insert the audit and redirect rows. If anything fails, roll back and call
   `journal.Rollback()`. The old files are untouched, so nothing is lost. A
   process killed here leaves only unreferenced new files, which the next run
   reuses.
5. After commit, unlink the old files with `RemoveCommittedOrphan`: only
   after commit, under the lock, and only when no entry references the path.
   If the process dies here, the old files stay listed in `old_files`, and the
   next diagnosis reports them and the next apply unlinks them.
6. Compact (§4).

**Idempotence.** A second run finds no plaintext at sensitive paths, no
superseded files and no pending compaction, so it reports `ok` and writes
nothing. Value search depends on step 4 committing atomically with the
payload rewrites: the values are collected from the old payloads before
those payloads are unlinked.

**Registry order.** The repair runs last, after `projections`. With a plain
diagnosis it reports the final classification's sensitive set. With
`--only sensitive-scrub`, it re-projects the affected current facts itself, so
it does not need the projections repair to run first.

## 6. Scope boundaries

The scrub does **not**:

- **Touch fact history.** Versions projected before the declaration keep the
  plaintext and the old entry ID, and as-of queries return them.
- **Touch exported bundles.** Bundles are not tracked. A bundle exported
  before the scrub still holds plaintext and old IDs, and restoring it brings
  them back; doctor will flag them again. The online backup copies every page,
  free pages included, so a bundle exported while compaction is still pending
  can carry old values.
- **Rewrite git history, backups, Time Machine or APFS snapshots.**
- **Shred deleted files.** Unlinking frees blocks, and on SSDs and copy-on-write
  filesystems, overwriting files proves nothing.
- **Delete import temp leftovers.**
- **Scrub non-JSON or compressed payloads.** It reports them with a
  delete-and-reimport fix.
- **Search for values shorter than 8 bytes** in unstructured artifacts.
- **Search for values whose payload is already gone.**
- **Redact facts added with `facts add`.**
- **Rename unchanged collection items** whose file names and origins embed a
  re-keyed collection's old ID.
- **Remove the hash oracle from old IDs** kept in redirects, history and
  reports (§2).

The scrub's output and `docs/cli/notes/doctor.md` state the remedy plainly:
**rotate every credential the scrub redacted**. The scrub limits further
exposure; it cannot undo exposure that already happened.

## 7. Delivery

One ticket (`pudl-m10`), delivered in one branch with the build and tests
green at each commit:

1. This revised design.
2. The shared record encoder (`importer.EncodeRecord`) and
   `projection.Correct`.
3. The audit migration, re-keying with merge, redirect lookup, report
   rewrites and compaction in `internal/database`.
4. `internal/scrub`: detection, value collection and search, the plan, file
   publication, the rewrite transaction, unlinking and compaction.
5. The `sensitive-scrub` repair, `Repair.OptIn`, doctor messaging, the
   redirect fallback in `lister.FindEntry`, docs and the help golden.

**Estimate:** the first draft estimated about 650 production lines (±150).
Re-derivation adds re-keying, merging, redirects and resolution, and drops the
in-place history rewrite and the dedup change. Expect about 900 production
lines and 500 test lines. The implementation log reports the measured net
delta.

## Acceptance

These are the ticket's criteria, made concrete:

- A payload imported before its schema gained a sensitive field is flagged,
  along with its collection parent, its `.meta`, its current projected facts,
  and a run report that quotes the value. The JSON and text output never
  contain the value.
- `--fix --dry-run --only sensitive-scrub` writes nothing. Apply redacts every
  flagged artifact.
- After apply, all of these succeed: the diagnosis returns `ok`;
  `--verify-payloads` passes; each scrubbed entry's ID is the hash import
  would compute, and no catalog column holds the old ID; `pudl show <old
  proquint>` resolves to the new entry; `pudl query` returns current facts with
  the new `entry_id` and no plaintext; `pudl facts list --as-of-tx` (at a time
  before the scrub) returns history unchanged; re-importing the original source
  deduplicates; `identity_json` is unchanged; and a byte search of
  `catalog.db` and `catalog.db-wal` for the value finds it only in fact
  history, if it was projected before the declaration.
- A process killed after file publication and before commit leaves the
  catalog unchanged, and the next apply completes. A process killed after
  commit and before unlink, or before compaction, is reported by the next
  diagnosis and finished by the next apply.

## Decisions (2026-11-01)

1. **IDs: re-derive.** This overrides the recommendation. A scrubbed entry gets
   a new content-derived ID from its redacted bytes, so the ID no longer acts
   as a brute-force oracle for the secret. The scrub must therefore:
   - rewrite every reference to the old ID in one transaction: collection
     memberships, snapshot membership and pins, `report_snapshot_refs`,
     projection state and provenance, version history, and any other column
     that stores an entry ID;
   - merge rather than fail when the new ID already exists, for example
     because the source was re-imported after the field became sensitive;
   - record old-to-new ID mappings in the audit table, so stored reports
     that cite old IDs stay resolvable.
   `resource_id` is unchanged, because identity fields cannot be sensitive.
2. **Fact history: leave it alone.** This overrides the recommendation. The
   append-only invariant stands. Current projected facts are corrected by
   re-projection. Historical fact versions keep their plaintext and their
   IDs. Doctor states this limitation and recommends rotating the credential.
3. **Opt-in: only when named.** Plain `--fix` never scrubs. It runs only with
   `--fix --only sensitive-scrub`, which is the confirmation. There is no
   prompt.
4. **Unstructured artifacts: search and redact.** Substring-replace each
   collected value that is 8 bytes or longer in reports, `.meta`, legacy mu
   files and snapshot files. Report how many shorter values were skipped.

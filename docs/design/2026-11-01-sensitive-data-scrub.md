# Sensitive-data scrub: redact plaintext stored before a field became sensitive

**Date:** 2026-11-01

**Status:** Accepted with changes on 2026-11-01; see
[Decisions](#decisions-2026-11-01). Tracked by beads `pudl-m10`. Follows
[GCP cataloging UX](2026-10-08-gcp-cataloging-ux.md) §6.2 ("Retroactive").
The user overrode two recommendations: IDs are **re-derived**, and fact history
is **left unchanged**. Where the body below says otherwise, the decisions
section governs.

## Decision

Add one opt-in `pudl doctor` repair, `sensitive-scrub`.

- **Diagnosis** runs with every `pudl doctor`. It reports stored plaintext at
  paths that a schema now declares in `_pudl.sensitive_fields`. It names entry
  IDs, fact relations, report IDs and field paths, and never prints values.
- **Apply** runs only when named:
  `pudl doctor --fix --only sensitive-scrub`. Add `--dry-run` to preview. It
  rewrites each affected artifact so that it holds what a fresh import would
  store today: `"[REDACTED]"` at each sensitive path. It then compacts SQLite
  so that the old values do not survive in the WAL or in free pages.
- **Entry and fact IDs are kept.** The scrub updates `content_hash`,
  `stored_path` and `size_bytes`, and records an audit row for each change.
  References from facts, snapshots, reports and memberships keep resolving.
- **Fact history is rewritten in place**, under one audited exception to
  append-only transaction time. The exception covers only values at sensitive
  args.

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
  memory only, and searches the remaining artifacts for them as substrings.
  Only values of at least 8 bytes are searched, because shorter ones (`true`,
  `admin`) would redact unrelated text. The scrub counts the shorter values
  it skipped and reports that count.

| Artifact | How plaintext got there | Detect | Scrub |
|---|---|---|---|
| Document payload (JSON) | Imported before the declaration | Path, under the entry's schemas | Re-encode the redacted record exactly as the import transform does |
| Collection item / observe record | Same | Path, under the item's schemas | Write `json.Indent(json.Marshal(redacted))`, as the collection stream does |
| Collection parent file | Holds every item record | Flagged when any member item is flagged | Re-encode each record (NDJSON or array framing, as in the transform), redacted under its member item's schemas, using `collection_memberships.item_index` |
| Non-JSON payload (YAML, CSV) | Imported before the declaration; import now fails closed for these | Path, after `DecodeStoredFile` | **Not scrubbed.** Reported with the fix "delete the entry and re-import as JSON". Import refuses to store these formats unredacted, so the scrub does not convert them either. |
| `.meta` file | `validation_errors` and trace reasons quote CUE values | Flagged with its entry, plus a value search | Replace each `validation_errors` message with import's withheld message (`stripValidationValues`), update `content_hash`, then run the value search over the rest |
| Projected facts, current **and history** | Projected before the declaration; correction closed but kept the rows | Path: a projected arg is sensitive when the relation's `each` path joined with the arg's path overlaps a sensitive path of the schema named in the row's `provenance` (`{"entry_id","schema"}`). Checked in SQL over `facts` and `current_facts` | Set the arg to `"[REDACTED]"` in place (§3) |
| `current_facts_fts` | FTS5 segments keep tokens after rows are deleted | Implied by any fact change | `INSERT INTO current_facts_fts(current_facts_fts) VALUES('rebuild')` |
| `run_reports`, `run_set_reports` JSON | Check witnesses (fact args), and drift `expected`, `observed` and `previous.value` | Value search | Rewrite matching strings with an `UPDATE` (not `SaveRunReport`, which resets pins) |
| Legacy `run_approvals` and `run_set_approvals` `request_json` | Convergence-era plans and diffs | Value search | Same as reports |
| Legacy `pudl/mu.#DriftObservation`, `#Manifest`, `#ManifestAction` and `#PlanOutput` entries | `pudl-bq5` and `pudl-8b7` stored them verbatim | Value search (manifest outputs have no schema, so the path method cannot apply) | Rewrite the file like a payload |
| Observe snapshot files | Summary and per-record `errors` strings | Value search | Rewrite the file like a payload |
| SQLite WAL and free pages | Old page images, plus deleted `current_facts` and FTS rows | Not scanned. A scrub records whether compaction finished (§5), and doctor reports a scrub whose compaction is pending | §4 |
| Superseded files from an interrupted scrub | Crash after commit, before unlink | The audit table lists the old paths; any that still exist are findings | Unlink them |
| `data/tmp` and `.staging-*` leftovers | A SIGKILLed import (§6.2) | Count and age only | **Report only.** A concurrent import's temp files live there before it takes the lock, so the scrub must not delete them. |
| Exported bundles and backups | Outside the workspace | Cannot detect | Out of scope (§6) |

**Identity is unaffected, and the scrub verifies it.** `redact` already makes
a sensitive path that overlaps the schema's own identity fields a load error.
For extra safety, the scrub also recomputes each entry's identity values from
the redacted record and refuses that entry if `identity_json` would change.
So `resource_id`, versions and projection sources stay as they are.

**Values are never printed.** Findings carry the subject kind and ID, the
schema or relation, and the path or arg name. Collected values live only in
process memory. Dry-run, JSON output, error messages and the audit tables
never contain them.

**Placement.** The per-entry path check runs inside the existing catalog scan
(`cmd/doctor_catalog.go`), which already decodes each payload for the secret
scan (`pudl-udi`). It passes the flagged entry IDs to the repair through
`repair.Options`, in the same way as `DriftEntryIDs`. The repair's `Diagnose`
runs the facts SQL and, only when some entry is flagged, reloads the flagged
payloads to collect values and searches reports, `.meta` files and legacy
files. A clean catalog pays only for the per-entry path check and one
indexed facts query.

## 2. Content addressing

The entry ID is the SHA-256 of the stored bytes at first catalog time.
`content_hash` drives dedup (`FindByContentHash`), `VerifyPayloads` and bundle
export. A scrub changes the bytes. There are two options.

**A. Re-derive IDs.** The new ID is the hash of the redacted bytes, and every
reference is rewritten.

**B. Keep IDs (recommended).** The scrub sets `content_hash`, `size_bytes`
and `stored_path`/`metadata_path` from the redacted bytes, and records a
`sensitive_redactions` row. For scrubbed entries, the ID stops being the hash
of the current bytes. It is still the hash of the bytes first cataloged.

| Concern | A: re-derive | B: keep |
|---|---|---|
| Dedup of a re-import of the same source | The import transform redacts, so its hash equals the new ID: works. But if that source was re-imported *after* the declaration, an entry with this ID already exists, causing a primary-key collision and a forced merge | `content_hash` equals the transform's hash, so `FindByContentHash` matches: works. Two entries may then share a `content_hash` (the index is not unique, and that is allowed). Nothing is merged |
| Entries whose content becomes identical (they differed only in the secret) | Primary-key collision; merge required | Both kept; same `content_hash` |
| `resource_id` | Unchanged for identity schemas; for identity-less schemas, it is derived from the hash and must be recomputed | Unchanged. The `identity` repair does not recompute it (`migrateIdentities` acts only on leaf-namespaced IDs) |
| Version history | IDs change within the chain | Unchanged. A version whose only change was the secret becomes byte-identical to its predecessor, and both are kept |
| Snapshot membership and `collection_memberships` | Rewrite `item_id` and `collection_id` | Unchanged |
| `projection_state.entry_id`, projected `entry_id` args, provenance, `item_schemas`, `resource_observations` | Rewrite them all, including fact history | Unchanged |
| Reports (`evidence_ids`, `snapshot_id`), `report_snapshot_refs`, pins | Dangling unless rewritten inside report JSON | Unchanged. Snapshot IDs are pre-allocated, not content-derived, in both options |
| Proquints users have noted down | Break | Resolve |
| Bundle export and verification | Works (manifest hashes are file byte hashes) | Works: `VerifyPayloads` checks the new `content_hash` |
| Old bundles | Hold old IDs and plaintext; self-consistent | Same |
| Local hash oracle | Removed from the catalog (except in old bundles) | **Kept.** The entry ID, and a fact ID (`ComputeFactID` hashes the args), is SHA-256 over content in which only the secret is now unknown. A low-entropy secret, such as a PIN or a dictionary password, can be brute-forced from these IDs |

**Recommendation: B.** A breaks provenance broadly, and it needs merge logic
for a case that will happen: a source re-imported after the declaration.
B's cost is the hash oracle, which matters only for low-entropy secrets. Those
secrets must be rotated anyway, because the plaintext was at rest and may sit
in bundles and backups (§6). The scrub's output says so every time it
redacts anything.

Two small changes make B airtight:

- **Dedup also matches by ID.** `findByContentHashIn` adds `OR id = ?`.
  Without this, a source whose bytes still hash to a scrubbed entry's ID (for
  example, after the field is un-declared) would fail on the primary key
  instead of deduplicating.
- **The encoder is shared.** The import transform's per-record encoding and
  collection framing move into one function that both the transform and the
  scrub call. Then "what a fresh import would store" holds by construction,
  and dedup matches byte for byte.

## 3. Append-only facts

History rows in `facts` hold plaintext args. There are three options:

1. **An audited in-place rewrite (recommended).** Rewrite the args of every
   row (`facts` and `current_facts`), replacing each sensitive arg with
   `"[REDACTED]"`. Record a `sensitive_redactions` row (`subject_kind=fact`)
   in the same transaction. IDs, relation, valid and transaction times,
   `tx_seq`, `supersedes`, source and provenance never change.
2. **Tombstoning.** Delete the rows and insert markers. As-of queries then
   lose facts, `supersedes` chains break, and fact versions have gaps. It
   changes *which* facts history returns, which is worse than changing a value.
3. **Leave history and document it.** `pudl facts list --as-of-tx` would keep
   serving the secret. This fails the ticket's goal.

**The invariant changes as follows.** `docs/facts.md` ("Transaction time is
append-only") and `docs/library-api.md` currently say that no operation
rewrites what the store believed at an earlier moment. The new rule:

> No operation rewrites what the store believed at an earlier moment, with one
> exception: the sensitive scrub replaces values at sensitive projected args
> with `"[REDACTED]"` in every version. An as-of query then returns the
> placeholder for those args. It returns the same facts, intervals, IDs and
> sequence numbers as before. Each rewritten row has an audit row. For
> rewritten rows, the fact ID no longer equals `ComputeFactID` of the stored
> args.

There are two consequences:

- `ReconcileProjection` compares facts by content, not by ID, so a scrubbed
  current row does not churn.
- If someone re-adds a scrubbed fact's plaintext with the same `valid_start`,
  AddFact rejects it, because the store rejects reusing an ID for different
  content. Projection never re-adds plaintext, so this cannot happen through
  normal use.

Facts from `pudl facts add` have no schema and are out of scope. The secret
scan covers them.

## 4. SQLite

After the rewrite transaction commits, the scrub runs these steps on one
pinned connection (`db.Conn`), still holding the artifact lock:

1. Before the transaction, `PRAGMA secure_delete=ON` on that connection. This
   zeroes the bytes freed by the scrub's own updates. It covers the window
   before `VACUUM`, and the case where `VACUUM` cannot run. Its cost is extra
   writes, during a rare operation.
2. The FTS `rebuild` (§1).
3. `PRAGMA wal_checkpoint(TRUNCATE)`, which copies the new pages over the old
   ones and truncates the WAL.
4. `VACUUM`. This is the only step that removes plaintext left in free pages
   by *earlier* deletes, such as the `current_facts` and FTS rows deleted when
   the projections repair re-projected. `VACUUM` makes a logical copy. The
   scrub has already committed by then, so the temporary copy holds no
   plaintext.
5. `PRAGMA wal_checkpoint(TRUNCATE)` again, because in WAL mode `VACUUM` writes
   through the WAL. Then set `compacted_at`.

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
  busy timeout. In that case the scrub returns `partial` with the message
  "compaction pending". Doctor keeps reporting it until a rerun completes it.

Compaction runs only when the scrub rewrote rows in the database (facts or
reports). A scrub that changed only files skips it.

## 5. Workflow and safety

**Commands.**

- `pudl doctor` diagnoses: status `needed`, with the subject list and the
  preview and apply commands from `repairArgv`.
- `pudl doctor --fix` skips the scrub. Its status is `skipped`, with the
  message "opt-in: name it with --only".
- `pudl doctor --fix --dry-run --only sensitive-scrub` previews: counts per
  artifact kind, subjects and paths, files to unlink, and whether it will
  compact.
- `pudl doctor --fix --only sensitive-scrub` applies.

The scope flags `--entry`, `--schema` and `--origin` select payloads. Values
collected from the selected payloads are still searched for everywhere.

**Confirmation.** Naming the repair is the confirmation. There is no
interactive prompt, so agents and scripts work the same way as people.
Implementation: the repair gets an `OptIn bool` field. `Select` still returns
the repair for diagnosis, and `runDoctorRepairs` applies it only when it was
named in `--only`.

Findings do not fail `pudl doctor`, which matches the other repairs' `needed`
status. A failed apply returns `partial` or `error`, which fails `--fix`, as
it does today.

**Audit record.** A migration adds two tables. They hold no values.

```sql
CREATE TABLE IF NOT EXISTS sensitive_scrubs (
  scrub_id     TEXT PRIMARY KEY,     -- random proquint
  scrubbed_at  TIMESTAMP NOT NULL,   -- written in the rewrite transaction
  compacted_at TIMESTAMP,            -- set after step 5 of section 4
  summary      TEXT NOT NULL         -- JSON counts per subject kind, plus short values skipped
);
CREATE TABLE IF NOT EXISTS sensitive_redactions (
  scrub_id     TEXT NOT NULL,
  subject_kind TEXT NOT NULL,        -- entry | fact | run_report | run_set_report | approval
  subject_id   TEXT NOT NULL,
  paths        TEXT NOT NULL,        -- JSON: field paths, arg names, or "value-match"
  old_files    TEXT,                 -- JSON: superseded stored and metadata paths
  PRIMARY KEY (scrub_id, subject_kind, subject_id)
);
```

`pudl show <entry>` gains one line, "redacted after import on <date>
(<paths>)", read from these tables. `CatalogEntry` gets no new column, so the
SELECT and Scan lists do not change.

**Crash safety.** This reuses the import and delete patterns.

1. Take `artifacts.WithLock`. Finish any earlier scrub first: unlink its
   `old_files` that still exist, then compact if `compacted_at` is null.
2. Plan the changes, and write each rewritten file into `data/tmp`.
3. Install the new files at **new** paths through `artifacts.Journal.Publish`.
   `Publish` never overwrites, so the old bytes stay in place until commit.
   The new names add `.r<hash8>` before the extension, because stored paths
   are opaque columns.
4. In one catalog transaction, update the entry columns, the fact args, the
   report JSON and the `.meta` paths, and insert the audit rows. If anything
   fails, roll back and call `journal.Rollback()`. The old files are
   untouched, so nothing is lost.
5. After commit, unlink the old files, following `DeletePlan`'s rule: only
   after commit, under the lock, and only when no other entry references the
   path. If the process crashes here, the old files stay listed in
   `old_files`, and the next run reports and unlinks them.
6. Run the steps in §4.

**Idempotence.** A second run finds no plaintext at sensitive paths, no
superseded files and no pending compaction, so it reports `ok` and writes
nothing. Value search depends on step 4 committing atomically with the
payload rewrites: the values are collected from the old payloads before
those payloads are unlinked.

**Registry order.** The repair runs last, after `projections`. With a plain
diagnosis it reports the final classification's sensitive set. With
`--only sensitive-scrub`, it rewrites `current_facts` itself, so it does not
need the projections repair to run first.

## 6. Scope boundaries

The scrub does **not**:

- **Touch exported bundles.** Bundles are not tracked. A bundle exported
  before the scrub still holds plaintext, and restoring it brings the
  plaintext back; doctor will flag it again. Bundles exported after the scrub
  has compacted are clean. The online backup copies every page, free pages
  included, so a bundle exported while compaction is still pending can carry
  old values.
- **Rewrite git history, backups, Time Machine or APFS snapshots.**
- **Shred deleted files.** Unlinking frees blocks, and on SSDs and copy-on-write
  filesystems, overwriting files proves nothing.
- **Delete import temp leftovers.** It reports them only (§1).
- **Scrub non-JSON payloads.** It reports them with a delete-and-reimport fix.
- **Search for values shorter than 8 bytes** in unstructured artifacts.
- **Search for values whose payload is already gone.** Projected facts are
  still covered, because they are checked by path.
- **Redact facts added with `facts add`.**
- **Merge entries whose content becomes identical.**
- **Hide the hash oracle in entry and fact IDs** (§2).

The scrub's output and `docs/cli/notes/doctor.md` state the remedy plainly:
**rotate every credential the scrub redacted**. The scrub limits further
exposure; it cannot undo exposure that already happened.

## 7. Delivery

There are three tickets. Each leaves the build and tests green, and each
includes its own doc updates.

1. **Detection (`pudl-m10.1`).** New package `internal/scrub`: path checks for
   payloads and collections, the facts SQL, and value search detection. The
   catalog-scan hook, `repair.Options.SensitiveEntries`, `Repair.OptIn`, and
   the `sensitive-scrub` diagnosis. About 260 production and 180 test lines.
2. **Apply (`pudl-m10.2`).** The shared record encoder; the file rewrites
   through the journal; one transaction for entries, facts, reports and
   `.meta`; the audit migration; unlinking after commit; dedup by ID; the
   `pudl show` line; and the invariant text in `docs/facts.md` and
   `docs/library-api.md`. About 320 production and 240 test lines.
3. **Compaction (`pudl-m10.3`).** `secure_delete`, the FTS rebuild, checkpoint
   and `VACUUM`, `compacted_at`, and the diagnosis of a pending scrub. Update
   §6.2 of the GCP design, the doctor notes, the CLI reference and the help
   golden. About 70 production and 60 test lines.

**Estimate:** about **650 production lines (±150)** and about 480 test lines.
Most of the production code is in the apply step, because each artifact kind
needs its own rewrite. Every implementation log reports the measured net
production-line delta.

## Acceptance

These are the ticket's criteria, made concrete:

- A payload imported before its schema gained a sensitive field is flagged,
  along with its collection parent, its `.meta`, its projected fact history,
  and a run report that quotes the value. The JSON and text output never
  contain the value.
- `--fix --dry-run --only sensitive-scrub` writes nothing. Apply redacts every
  flagged artifact.
- After apply, all of these succeed: the diagnosis returns `ok`;
  `--verify-payloads` passes; `pudl show <old proquint>`, `pudl query`,
  `pudl facts list --as-of-tx` (at a time before the scrub) and
  `pudl run report` resolve;
  re-importing the original source deduplicates; `identity_json` and
  `resource_id` are unchanged; and a byte search of `catalog.db` and
  `catalog.db-wal` for the value finds nothing.
- A process killed after commit and before unlink, or after commit and before
  compaction, is reported by the next diagnosis and finished by the next apply.

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

# Evidence integrity, explanations, and recovery

PUDL keeps observations, imported data, facts, and run reports locally. These
features make the retained evidence inspectable and portable without granting
execution authority to restored history.

## Imports and exports

JSON numbers keep their digits through import, observation ingestion, export,
identity extraction, and report replay. SQL queries retain their existing checked
numeric domain; retaining a value does not promise that SQLite can evaluate it.

`pudl import --path data.json --explain --json` shows schema candidates, heuristic
scores, rejection paths, the selected schema source, local/global shadowing,
explicit-schema fallback, and unavailable declarations. The score is a ranking
heuristic, not a calibrated probability. Ordinary imports persist the assignment
reason; detailed traces are opt-in. Reimport uses the retained original trace when
available rather than presenting today's classification as an earlier decision.
Collection output includes at most 32 item explanations and marks truncation.

Collection imports and observation ingestion prepare private artifacts and a
descriptor spool before taking the SQLite write transaction. Publication and
catalog commit share an artifact lock with deletion, pruning, and backup. A failed
commit removes only artifacts newly published by that operation. A killed process
may leave inert private files or unreferenced bytes; it cannot authorize a partial
snapshot. Resource versions and deduplication are decided atomically at commit.

Import and `mu ingest-observe` expose byte limits. Defaults are 64 MiB per JSON
record (per target envelope for observations), 1 GiB decoded input, and 2 GiB
preparation storage. Nonstreamed documents, including CSV/YAML documents, use the
record limit for their complete input. Compressed imports count decoded bytes and
their temporary copies. Limits fail explicitly; records are never truncated.
Increase `--max-record-bytes`, `--max-decoded-bytes`, or `--max-staging-bytes` for
larger trusted inputs. Preparation and lock waiting honor cancellation.

Export walks supported source formats and normalized collection membership, keeps
exact JSON numbers, and orders CSV columns deterministically. CSV supports scalar
object fields and rejects shapes it cannot represent. A selected entry that cannot
be read fails the export. `--allow-partial` publishes readable data but reports
incomplete output on stderr and exits nonzero. File destinations are replaced only
after serialization and closing succeed; stdout may already contain a prefix if
its writer fails. Invalid formats preserve an existing destination.

## Reports and evidence lifetime

Live inventory findings contain expected, observed, and previous values. The
previous observation must belong to the same model and workspace and an eligible
earlier successful run. Historical values are frozen into the report. States
distinguish available values, absent fields, no baseline, ambiguous identities,
incompatible identity contracts, and pruned evidence. JSON null remains a value.
Catalog replay without a single observation scope cannot invent a prior baseline.

Failed checks include the expectation, query scope, total count, and at most 20
deterministically ordered witnesses. Outside `--only` violations remain advisory.
A failed nonempty check reports missing evidence rather than fabricating a proof.
Oversized witness values are omitted with an explicit marker, and sealed-reference
redaction applies before persistence. Human reports present drift and failed checks
in the finding view; the existing JSON drift/check sections remain available.

`pudl run report [id]` adds current evidence availability to retained findings.
This does not recompute their historical previous values. A pruned reference is
visible as pruned in human and JSON output.

Snapshot pins have independent manual, approval, and retained-report owners.
Releasing one owner does not release another. Report evidence is protected for
30 days by default; reports themselves remain after their evidence expires.
Use `pudl snapshot retain ID` to pin important evidence indefinitely and
`pudl snapshot show ID --json` to inspect active owners and expiration.
Pruning protects the latest eligible successful observation per model/workspace,
rechecks ownership inside its transaction, and preserves shared records. Explicit
deletion cannot bypass active evidence protection. File cleanup happens after
catalog deletion commits; cleanup errors are reported separately.
Cleanup resolves raw/metadata paths through directory handles and refuses escaping
parent symlinks. An explicitly configured external prune data directory remains
its own removal boundary.

`pudl query RELATION --timeout 10s` bounds a query independently of `--mu-timeout`.
SQL evaluation, recursive rounds, transaction acquisition, and generated-project
locks honor cancellation. The recursion-round cap remains a separate bound.
Public Go consumers can use `Store.QueryContext` without changing existing callers.

## Backup and restore

```bash
pudl doctor --verify-payloads --health-only
pudl export --bundle ../pudl-backup.tar.zst
```

Bundles capture a consistent SQLite snapshot, referenced raw/metadata files,
configuration, schemas, rules, models, definitions, and populators. Exact byte
checksums protect every archive member. Authored files changing during capture
fail the operation. Generated execution workspaces and private temporary files are
excluded. References outside the state root and symlinks are rejected; external
executables, Mu projects, providers, and global fallback schemas are not bundled.
Use a self-contained workspace when those dependencies need to travel together.

The source payload audit checks available SHA256 claims, including canonical JSON
item hashes. Older opaque identifiers have no verifiable source hash, but their
files still receive exact bundle checksums. Legacy manifest rows advertising
nonexistent synthetic `.meta` files are normalized only in the private backup
catalog; the manifest records this repair and the source remains unchanged.

The default uncompressed bundle limit is 16 GiB; `--max-bundle-bytes` can increase
it for export or restore. The archive is written beside the destination and
published only on success. Keep the archive outside the captured state root.

Restore into a directory which has no `.pudl/` yet:

```bash
mkdir restored-project
cd restored-project
pudl init --from-bundle ../pudl-backup.tar.zst
```

Restore verifies member paths, lengths, checksums, catalog integrity, references,
and schema compatibility in a private sibling directory before publishing it.
Storage/configuration paths are rebased; original source paths and historical
reports remain history. Existing workspaces are never overwritten. Unsupported
versions, missing members, corruption, symlinks, and appended nonarchive data fail.

Pending approvals become rejected and require replanning. Resource status becomes
unknown. Restored observations remain readable history but cannot supply producer
bindings until a new live successful observation is recorded. Restoring a backup
does not claim live verification or execute an operation.

These checks detect damage and incomplete backups. They do not authenticate who
created a bundle; a bundle's checksums are supplied by its own manifest.

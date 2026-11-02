# Evidence: explanations, retention and backup

pudl keeps its observations, imported data, facts and check reports locally.
This page covers how to see why a record was classified, how imports and
exports treat your data, how long report evidence is kept, and how to back up
and restore a workspace. For checks and findings, see
[governance](governance.md).

## Why was a record classified this way?

```bash
pudl import --path data.json --explain --json
pudl import --path data.json --dry-run
```

`--explain` shows the schema candidates, their heuristic scores, rejection
paths, the selected schema's source, local-over-global shadowing,
explicit-schema fallback and unavailable declarations. The score ranks
candidates; it is not a probability. A collection explains at most 32 items
and marks the truncation. Ordinary imports store only the assignment reason.

`--dry-run` writes nothing and reports, per schema, what an import would
store: validation failures, unresolved identity, projected facts, likely
secrets, and records already cataloged.

## Imports and exports keep your data exact

- **Numbers** keep their digits through import, observation, export, identity
  and report replay. Queries have a narrower numeric domain; see the
  [numeric contract](library-api.md#numeric-query-contract).
- **Limits.** Defaults are 64 MiB per JSON record, 1 GiB of decoded input and
  2 GiB of preparation storage; a populate command's output counts against
  the staging limit. Non-streamed documents (CSV, YAML) use the record limit
  for the whole input, and compressed input counts decoded bytes. A limit
  fails the import; records are never truncated. Raise them with
  `--max-record-bytes`, `--max-decoded-bytes` and `--max-staging-bytes`.
- **Atomic commits.** An import or observation prepares its files privately
  and publishes them with the catalog commit. A failed or killed import
  cannot leave a partial snapshot; at most it leaves inert private files.
- **Exports** keep source formats, collection membership and exact numbers.
  CSV columns are sorted and only scalar fields are supported. An unreadable
  entry fails the export; `--allow-partial` writes what it can, reports the
  rest on stderr and exits nonzero. A file destination is replaced only after
  the export succeeds.

To print one entry's payload exactly as stored, for piping:
`pudl export --id <proquint> --format json`.

## Reports and evidence lifetime

A report freezes what a check found: expected, observed and previous values
for drift, and up to 20 witnesses per failed check. Oversized witness values
are omitted with a marker. Reading a report later with `pudl report` adds
whether each referenced snapshot is still available (or `pruned`); it never
recomputes the findings.

Snapshots are kept until you prune them. Three independent owners can pin a
snapshot against pruning:

- **a report**: a report's evidence is kept for 30 days after it is saved; the
  report itself is kept after that;
- **you**: `pudl snapshot retain ID` pins it until `--release`;
- **legacy approvals** from older workspaces.

Releasing one owner leaves the others.

```bash
pudl snapshot list --model gcp-firewalls
pudl snapshot show ID --json      # provenance, completeness, pin owners, expiry
pudl snapshot prune --model gcp-firewalls --keep 5 --older-than 720h --dry-run
```

Pruning never removes a pinned snapshot or the latest successful observation
of a model, and keeps records that another snapshot still cites. Explicit
deletion cannot bypass a pin either.

`pudl query RELATION --timeout 10s` bounds a query; Go callers use
`Store.QueryContext`.

## Backup and restore

```bash
pudl doctor --verify-payloads --health-only
pudl export --bundle ../pudl-backup.tar.zst
```

A bundle holds a consistent copy of the catalog, the raw and metadata files it
references, and the workspace's configuration, schemas, models, rules,
definitions, populators and vendored CUE packages. Every member carries a
checksum. The export fails if authored files change during capture, and
rejects symlinks and references outside the workspace. External executables,
cloud credentials and personal global schemas are not bundled; keep a
workspace self-contained if they must travel with it.

The default limit is 16 GiB uncompressed, raised with `--max-bundle-bytes`.
Write the bundle outside the workspace. It is published only on success.

Restore into a directory that has no `.pudl/` yet:

```bash
mkdir restored-project
cd restored-project
pudl init --from-bundle ../pudl-backup.tar.zst
```

Restore verifies paths, lengths, checksums, catalog integrity, references and
schema compatibility in a private directory before publishing it, and never
overwrites an existing workspace. Restored observations are history: model
status becomes `unknown`, and retention does not count a restored snapshot as
a model's latest observation until the model is observed again.

Checksums detect damage and incomplete bundles. They do not authenticate who
made a bundle, because the manifest supplies them.

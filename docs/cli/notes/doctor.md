**Diagnose, then repair.** Plain `doctor` writes nothing. It checks
workspace health (schema packages that fail to load, `base_schema` cycles,
invalid Datalog rules with their file positions), validates catalog records
against their schemas, scans for likely secrets, and lists each repair that is
needed. `--fix` applies the needed repairs in the order listed in `--help` and
re-checks the catalog; `--fix --only NAME` limits them; `--fix --dry-run`
previews them. `--fix` refuses to write when a health check failed. A repair
that changes entries also resyncs projected facts.

**Scope.** `--entry` checks one record and cannot be combined with
`--health-only`. `--schema` and `--origin` scope the catalog scan and the
inference repair. The inference repair reclassifies only inferred entries the
scan found drifted; `--include-explicit` re-infers every entry in scope,
explicit assignments included. The identity repair backfills missing
`resource_id`s and recomputes those an older release namespaced by the leaf
schema.

**`versions`** renumbers each resource whose history holds two entries with
the same version, 1..N in import order. Observation ingest before pudl-42w
recorded every observed entry as version 1, so a resource observed in several
states (a model whose definition changed, say) has several version-1 entries.
It runs only when named: `--fix --only versions`.

**Likely secrets.** The scan reports likely secrets (known token formats and
a conservative entropy check) in stored payloads outside `sensitive_fields`
and `secret_scan_ignore`, and in the args of current facts, skipping facts
projection copied from payloads. Findings name the entry or fact, path and
kind, never the value. They are warnings and do not change `ok` or the exit
status.

**`sensitive-scrub`** removes plaintext stored before a schema declared the
path in `_pudl.sensitive_fields`, from payloads, collection and metadata
files, snapshot files, stored reports and legacy files. It runs only when
named: `--fix --only sensitive-scrub` (with `--dry-run` to preview). It
rewrites each artifact as a fresh import would store it, re-derives the ID of
each changed entry (old IDs and proquints still resolve), re-projects facts,
and compacts SQLite. Non-JSON or compressed payloads are listed as
unsupported: delete and re-import them. Fact history and exported bundles keep
the plaintext, so rotate every credential the scrub redacts.

**JSON.** `report_version` 3 with `mode` (`diagnose`, `dry-run` or `fix`),
`ok`, `health`, `catalog`, `secrets` (absent when the catalog was not
scanned), `repairs` and any setup `error`. Each repair has `name`,
`condition`, `status` (`ok`, `needed`, `applied`, `partial`, `error` or
`skipped`), `message`, `pending`, `changed`, `failed`, `changes`, `errors`,
optional `details`, and, when needed, `preview_argv` and `apply_argv`.
Invalid records, inference mismatches, failed health checks, and an `error`
or `partial` repair exit nonzero. An empty catalog is valid.

`--verify-payloads` checks referenced files and their SHA256 content claims.
It never repairs evidence.

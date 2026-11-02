`--entry` and `--health-only` are mutually exclusive. JSON includes
`report_version` (2), `mode` (`diagnose`, `dry-run` or `fix`), `ok`, `health`,
`catalog`, `repairs`, and any setup `error`; invalid records, inference
mismatches, and failed health checks produce a nonzero exit status. An empty
catalog is valid. Version 1 reports had no `report_version` and listed repairs
only as `reason`, `preview_argv` and `apply_argv`.

Each `repairs` item has `name`, `condition`, `status` (`ok`, `needed`,
`applied`, `partial`, `error` or `skipped`), `message`, `pending`, `changed`,
`failed`, `changes` (`subject`, `from`, `to`), `errors`, optional `details`
(the projections repair's sync report), and, when needed, `preview_argv` and
`apply_argv`. Plain `doctor` writes nothing. `--fix` applies the repairs in
registry order and re-checks the catalog afterwards; `--fix --dry-run` previews
them. `--fix` refuses to write when a health check failed, and an `error` or
`partial` repair fails the command. A repair that changes entries also resyncs
projected facts unless `projections` ran.

`--schema` and `--origin` scope the catalog scan and therefore the inference
repair. That repair re-infers only entries the scan found drifted; explicit
producer assignments are never reclassified unless `--include-explicit` asks
for every entry in scope (`--entry`, or `--schema`/`--origin`, or the whole
catalog). The identity repair backfills missing `resource_id`s and recomputes those that
still equal the leaf-schema-namespaced id, re-sequencing the versions of the
resources they join.

Health checks include schema loading (each schema package that fails to load,
and any `base_schema` cycle) and Datalog rules (each rule that cannot be
loaded, with its file position).

`--verify-payloads` checks referenced files and available SHA256 content claims, including canonical JSON items. This optional full scan never repairs or rewrites evidence.

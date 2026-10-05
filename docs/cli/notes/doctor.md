`--entry` and `--health-only` are mutually exclusive. JSON includes `ok`,
`health`, `catalog`, and any setup `error`; invalid records, inference
mismatches, and failed health checks produce a nonzero exit status. An empty
catalog is valid.

Health checks include schema loading (each schema package that fails to load,
and any `base_schema` cycle) and Datalog rules (each rule that cannot be
loaded, with its file position).

The `mu` health check runs `mu version` and warns when mu is missing, its
version cannot be read, or it is older than the minimum this PUDL is tested
against (v0.3.5). mu is optional for imports, queries and `--from-catalog`
replays, so these are warnings rather than failures.

`--verify-payloads` checks referenced files and available SHA256 content claims, including canonical JSON items. This optional full scan never repairs or rewrites evidence.

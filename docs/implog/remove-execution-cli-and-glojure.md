# Remove execution-related CLI commands and Glojure adapter

Removed CLI commands and code related to the execution layer that was extracted to mu.

## Removed

- `op/adapter.go` — `GlojureFunc` adapter and `GlojureCaller` interface (Glojure runtime bridge)
- `cmd/data.go` — parent `pudl data` command
- `cmd/data_search.go` — `pudl data search` (search artifacts by definition/method)
- `cmd/data_latest.go` — `pudl data latest` (show most recent artifact)
- `cmd/drift_check.go` — removed `--method` flag and method display from drift check output

## Kept

- `op/functions.go` — `CustomFunction` interface stays as part of the op package
- `internal/database/` artifact fields (Method, RunID, etc.) — left for future cleanup
- `internal/importer/bootstrap/pudl/model/` CUE schemas — left for future cleanup
- `internal/drift/` internals — `CheckOptions.Method` field left in place

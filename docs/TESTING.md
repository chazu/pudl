# PUDL Testing Documentation

Overview of the testing strategy for PUDL.

## Test Categories

| Category | Location | What it covers |
|----------|----------|----------------|
| Database | `internal/database/` | CRUD, queries, collections, migrations, concurrent access |
| Importer | `internal/importer/` | Format detection, schema assignment, collections, streaming, error handling |
| Inference | `internal/inference/` | Heuristic scoring, CUE unification, inheritance graph |
| Validator | `internal/validator/` | CUE validation, validation service |
| Run/CLI | `cmd/` | Run flags, inventory snapshots, CLI orchestration, legacy catalog readability |
| Observe | `internal/observe/` | Observe snapshots, schema routing, and redaction |
| Identity | `internal/identity/` | Resource identity extraction, content hashing |
| Schema Name | `internal/schemaname/` | Normalization, canonical format |
| Integration | `test/integration/` | End-to-end import-to-catalog workflows |
| System | `test/system/` | Reliability, config, edge cases, stress tests |
| Getting-started smoke | `test/smoke/git_inventory_walkthrough_test.go` | Documented Git inventory commands, drift, snapshot evidence, and isolation |
| Example smoke | `test/smoke/example_install_test.go`, `test/smoke/git_inventory_observation_test.go` | Embedded example installation, conflict preservation, and live command observation |

## Running Tests

The repository targets Go 1.26.9, pinned in `mise.toml` and CI. Run
`mise install` once, then use `mise exec --` before the Go and Make commands
below to select the project toolchain. With mise activated in your shell,
the toolchain is selected automatically inside this checkout.

The CI quality gate also runs the generated skill check and uses the explicit
`checkptr` exception below because the CDC dependency uses unsafe pointer
arithmetic.

### All tests
```bash
go test ./...
```

### Repository-local isolation smoke test

Run these from a disposable or intentionally dogfooded repository checkout:

```bash
mise exec -- go run . repo init
mise exec -- go run . config
mise exec -- go run . doctor
```

`config` must report `<repo>/.pudl` as the workspace, schema, data, and config
root. After an import, raw content, metadata, and `data/sqlite/catalog.db` must
appear only beneath that directory. Hash or timestamp the global
`~/.pudl/data/sqlite/catalog.db` before and after when explicitly testing the
no-leak boundary. `repo init` is safe to repeat and should repair missing
PUDL-owned files without replacing an authored `workspace.cue` unless
`--force` is passed.

### Git inventory walkthrough

```bash
mise exec -- make test-git-walkthrough
```

Requires Git. The test builds a fresh PUDL binary, places it, Git, `cat` and
`cp` on an otherwise empty `PATH`, and executes the marked commands from
[getting-started.md](getting-started.md). No shell scripts or copied source
fixtures are used. Printed ID placeholders are filled from CLI results as the
guide instructs the reader. It verifies the clean baseline, changed inventory,
repeat finding, durable report replay, typed evidence, and absence of global
PUDL state. The observation journey repeats baseline/change/repeat runs of the
installed example and checks typed snapshot members and retained evidence.
Installer checks cover idempotence, conflicts, and use outside an initialized
workspace. Paths include spaces, and test workspaces below
`.pudl/data/git-walkthrough/test-runs/` are removed afterward.

CI runs this check in the ordinary test job; it needs no mu or Python.

### By category
```bash
# Database
go test ./internal/database -v

# Importer
go test ./internal/importer -v

# Inference
go test ./internal/inference -v

# Validator
go test ./internal/validator -v

# Observation ingest
go test ./internal/observe -v

# Integration
go test ./test/integration/... -v

# System reliability
go test ./test/system -v
```

### With race detection
```bash
go test -race -gcflags=all=-d=checkptr=0 ./...
```

### With coverage
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Architecture

### Directory structure
```
internal/
  database/       *_test.go   -- CRUD, queries, collections, migrations
  importer/       *_test.go   -- Import pipeline, format detection, collections
  inference/      *_test.go   -- Schema inference, heuristics, CUE unification
  validator/      *_test.go   -- CUE validation
  mubridge/       *_test.go   -- Typed envelopes, observe snapshots, manifest ingest
  identity/       *_test.go   -- Resource identity
  schemaname/     *_test.go   -- Name normalization
  schemagen/      *_test.go   -- Schema generation

test/
  fixtures/           Test data and utilities
  testutil/           Shared test helpers
  integration/        End-to-end tests
    infrastructure/   Test framework (suite, data generation)
    workflows/        Import workflow tests
  system/             System-level tests (config, reliability, edge cases)
  smoke/              Build-tagged real CLI and external-tool tests
```

### Test data strategy
- All test data is synthetic -- no external dependencies
- Realistic scenarios using AWS, Kubernetes, and generic data patterns
- Database tests use `os.MkdirTemp` + `NewCatalogDB(tmpDir)` pattern
- Backfill tests close and reopen the DB to trigger migrations

## Key Test Areas

### Database Tests
- CRUD operations with validation and error handling
- Query engine: filtering, sorting, pagination, complex combinations
- Collection parent-child relationships and cascade operations
- Many-to-many collection membership and shared-item deletion semantics
- Concurrent multi-threaded safety
- Migration idempotency

### Importer Tests
- Format detection (JSON, YAML, NDJSON, CSV)
- Schema inference assignment and confidence scoring
- Large file streaming and memory efficiency
- Collection imports fail atomically when an item cannot be stored

- Error handling for corrupted and malformed input

### Inference Tests
- Heuristic scoring across resource types
- CUE unification with schema candidates
- Inheritance graph traversal and specificity ordering

### System Model Tests
- `#SystemModel` schema decode + structural validation (`internal/systemmodel`)
- Run verdict mapping and the per-model phase plan of `pudl check` (`cmd` run tests)
- Retired model fields fail to load with an error naming the field
  (`internal/systemmodel`)
- `pudl check` ordering over `#CommandObserve` members (producers first by
  `depends_on`, blocked dependents), the one versioned check report with
  embedded member reports, and durable linked reports
  (`cmd/check_integration_test.go`)
- A catalog written before the convergence/mu removal still lists, shows,
  reports and validates (`cmd/legacy_catalog_test.go`)
- Bootstrap installation and repair derived from the complete embedded schema
  tree

### Environment-sensitive tests

The database initialization test intentionally exercises an unwritable path;
its assertion is about returning an error, not a particular host `errno` string.
The `make smoke` suite skips cases whose external tools are unavailable.

### Observe Tests
- Observe-result ingestion into the catalog (content-hash dedup, schema routing)
- Sensitive-field redaction before storage

### Integration Tests
- End-to-end file-to-database import workflows
- Multi-format processing in single test runs
- Error handling: corrupted data, empty files, invalid paths

### System Tests
- Database initialization under various directory states
- Concurrent stress testing (multiple workers, thousands of operations)
- Resource exhaustion and graceful degradation
- Edge cases: empty databases, boundary values, error propagation

The full race gate keeps all 300 differential-oracle seeds. It uses a 20-minute
package timeout and two concurrent packages because pure-Go SQLite plus race
instrumentation is substantially slower than ordinary tests. `make test-race`
and CI use the same command; pointer checks and race detection remain enabled.

Evidence acceptance also covers portable bundles, payload corruption, historic
report values, check witnesses, retention owners, cancellation, prepared
collections, and the actual Git example CLI journey. The native
`internal/ingestprep` array framer has a differential JSON fuzz target included
in `make fuzz`.

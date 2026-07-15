# PUDL Testing Documentation

Overview of the testing strategy for PUDL.

## Test Categories

| Category | Location | What it covers |
|----------|----------|----------------|
| Database | `internal/database/` | CRUD, queries, collections, migrations, concurrent access |
| Importer | `internal/importer/` | Format detection, schema assignment, collections, streaming, error handling |
| Inference | `internal/inference/` | Heuristic scoring, CUE unification, inheritance graph |
| Validator | `internal/validator/` | CUE validation, validation service |
| Run/CLI | `cmd/` | Run flags, scoped convergence, inventory snapshots, CLI orchestration |
| Mubridge | `internal/mubridge/` | Observe snapshots, envelopes, and manifest ingest |
| Identity | `internal/identity/` | Resource identity extraction, content hashing |
| Schema Name | `internal/schemaname/` | Normalization, canonical format |
| Integration | `test/integration/` | End-to-end import-to-catalog workflows |
| System | `test/system/` | Reliability, config, edge cases, stress tests |

## Running Tests

The repository targets Go 1.25.8. The CI quality gate also runs the generated
skill check and uses the explicit `checkptr` exception below because the CDC
dependency uses unsafe pointer arithmetic.

### All tests
```bash
go test ./...
```

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

# Mu bridge
go test ./internal/mubridge -v

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
- Typed envelope detection, schema metadata, and inline definition caching
- Error handling for corrupted and malformed input

### Inference Tests
- Heuristic scoring across resource types
- CUE unification with schema candidates
- Inheritance graph traversal and specificity ordering

### System Model Tests
- `#SystemModel` schema decode + structural validation (`internal/systemmodel`)
- Run verdict mapping, `--only` scope selection, and the `pudl run` phase plan (`cmd` run tests)

### Environment-sensitive tests

The database initialization test intentionally exercises an unwritable path;
its assertion is about returning an error, not a particular host `errno` string.
Integration tests that require external `mu`/plugin binaries are explicitly
skipped when those tools are unavailable.

### Mubridge Tests
- Observe-result ingestion into the catalog (content-hash dedup, schema routing)
- Build-manifest ingestion + per-action catalog entries and status

### Integration Tests
- End-to-end file-to-database import workflows
- Multi-format processing in single test runs
- Error handling: corrupted data, empty files, invalid paths

### System Tests
- Database initialization under various directory states
- Concurrent stress testing (multiple workers, thousands of operations)
- Resource exhaustion and graceful degradation
- Edge cases: empty databases, boundary values, error propagation

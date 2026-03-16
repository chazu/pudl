# PUDL Testing Documentation

Overview of the testing strategy for PUDL.

## Test Categories

| Category | Location | What it covers |
|----------|----------|----------------|
| Database | `internal/database/` | CRUD, queries, collections, migrations, concurrent access |
| Importer | `internal/importer/` | Format detection, schema assignment, collections, streaming, error handling |
| Inference | `internal/inference/` | Heuristic scoring, CUE unification, inheritance graph |
| Validator | `internal/validator/` | CUE validation, validation service |
| Definition | `internal/definition/` | Discovery, schema reference parsing, dependency graph, validation |
| Drift | `internal/drift/` | JSON deep diff, drift checker, report storage |
| Mubridge | `internal/mubridge/` | Drift-to-action export, plan response generation |
| Type Patterns | `internal/typepattern/` | AWS, Kubernetes, GitLab pattern detection |
| Identity | `internal/identity/` | Resource identity extraction, content hashing |
| Schema Name | `internal/schemaname/` | Normalization, canonical format |
| Integration | `test/integration/` | End-to-end import-to-catalog workflows |
| System | `test/system/` | Reliability, config, edge cases, stress tests |

## Running Tests

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

# Definitions
go test ./internal/definition -v

# Drift detection
go test ./internal/drift -v

# Mu bridge
go test ./internal/mubridge -v

# Type patterns
go test ./internal/typepattern -v

# Integration
go test ./test/integration/... -v

# System reliability
go test ./test/system -v
```

### With race detection
```bash
go test ./... -race
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
  definition/     *_test.go   -- Definition discovery, graph, validation
  drift/          *_test.go   -- Deep diff, checker, reports
  mubridge/       *_test.go   -- Action export
  typepattern/    *_test.go   -- Type detection patterns
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
- Concurrent multi-threaded safety
- Migration idempotency

### Importer Tests
- Format detection (JSON, YAML, NDJSON, CSV)
- Schema inference assignment and confidence scoring
- Large file streaming and memory efficiency
- Collection wrapper detection and unwrapping
- Error handling for corrupted and malformed input

### Inference Tests
- Heuristic scoring across resource types
- CUE unification with schema candidates
- Inheritance graph traversal and specificity ordering

### Definition Tests
- Discovery of definitions via CUE schema reference patterns
- Dependency graph construction from socket wiring
- Cycle detection and topological sort
- Self-contained test fixtures (no external schema dependencies)

### Drift Tests
- JSON deep diff with recursive field comparison
- Numeric type coercion and dot-notation paths
- Drift checker comparing definitions against catalog state
- Report storage: save, list, get, getLatest

### Mubridge Tests
- Drift report to action spec conversion
- Plan response generation for single and multiple definitions
- ListDefinitions enumeration

### Integration Tests
- End-to-end file-to-database import workflows
- Multi-format processing in single test runs
- Error handling: corrupted data, empty files, invalid paths

### System Tests
- Database initialization under various directory states
- Concurrent stress testing (multiple workers, thousands of operations)
- Resource exhaustion and graceful degradation
- Edge cases: empty databases, boundary values, error propagation

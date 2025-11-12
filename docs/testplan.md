# PUDL Comprehensive Testing Strategy

## Executive Summary

This document outlines a comprehensive testing strategy to improve PUDL's test coverage from the current 19.5% to 70%+ overall coverage, focusing on critical business logic, user-facing features, and error handling.

## Current State Assessment

### Existing Test Coverage
- **Total Coverage**: 19.5% across the entire project
- **Tested Packages**: Only 2 out of 12 internal packages have tests
  - `internal/rules`: 57.8% coverage (4 test files)
  - `internal/streaming`: 58.6% coverage (3 test files)
- **Untested Packages**: 10 critical packages with 0% coverage
- **CLI Commands**: 0% coverage across all 11 command files

### Critical Gaps Identified
1. **Core Business Logic**: No tests for importer, database, validator
2. **CLI Commands**: No command-line interface testing
3. **Configuration Management**: No config system tests
4. **Error Handling**: No error path testing
5. **Integration Workflows**: No end-to-end testing

## Priority Areas for Testing

### Priority 1: Critical Business Logic (High Impact)
1. **Data Import Pipeline** (`internal/importer/`)
   - File format detection and parsing
   - Schema assignment and validation
   - Metadata generation and catalog updates
   - Collection processing (NDJSON handling)

2. **Database Operations** (`internal/database/`)
   - Catalog CRUD operations
   - Migration from JSON to SQLite
   - Query performance and filtering

3. **Schema Validation** (`internal/validator/`)
   - CUE module loading and validation
   - Cascade validation logic
   - Schema resolution and fallback

### Priority 2: User-Facing Features (High Visibility)
4. **CLI Commands** (`cmd/`)
   - `init`, `import`, `list`, `show` commands
   - Error handling and user feedback
   - Configuration management

5. **Configuration System** (`internal/config/`)
   - Config loading, validation, and persistence
   - Path resolution and defaults

### Priority 3: Supporting Infrastructure (Medium Impact)
6. **Error Handling** (`internal/errors/`)
   - Error classification and formatting
   - Recovery mechanisms

7. **Git Integration** (`internal/git/`)
   - Repository operations and validation

8. **CUE Processing** (`internal/cue/`)
   - Custom function processing
   - AST manipulation

## Testing Strategy Design

### Unit Tests (Fast, Isolated)
**Target**: 80%+ coverage for business logic functions

**Focus Areas**:
- Pure functions with clear inputs/outputs
- Data transformation logic
- Validation rules and schema matching
- Format detection algorithms

### Integration Tests (Component Interactions)
**Target**: Key workflows and component boundaries

**Focus Areas**:
- Importer + Database + Validator integration
- CLI command execution with real file system
- Configuration loading with various scenarios
- Error propagation across components

### End-to-End Tests (Full Workflows)
**Target**: Critical user journeys only

**Focus Areas**:
- `pudl init` → `pudl import` → `pudl list` workflow
- Large file streaming import
- Schema validation with cascading fallback
- Migration scenarios

## Implementation Plan

### Phase 1: Foundation Testing (Week 1-2)
**Estimated Effort**: 16-20 hours

#### 1.1 Core Business Logic Tests
```
internal/importer/importer_test.go
internal/importer/detection_test.go
internal/importer/schema_test.go
internal/database/catalog_test.go
internal/database/migration_test.go
```

**Key Test Scenarios**:
- File format detection (JSON, YAML, CSV, NDJSON)
- Schema assignment with various confidence levels
- Database operations with SQLite backend
- Migration from legacy JSON catalog

#### 1.2 Configuration System Tests
```
internal/config/config_test.go
```

**Key Test Scenarios**:
- Default configuration loading
- Path validation and expansion
- Configuration persistence and loading
- Error handling for invalid configs

### Phase 2: CLI Command Testing (Week 3)
**Estimated Effort**: 12-16 hours

#### 2.1 Command Integration Tests
```
cmd/init_test.go
cmd/import_test.go
cmd/list_test.go
cmd/show_test.go
```

**Key Test Scenarios**:
- Command execution with temporary directories
- Error handling and user feedback
- Flag parsing and validation
- Integration with underlying services

### Phase 3: Validation & Advanced Features (Week 4)
**Estimated Effort**: 12-16 hours

#### 3.1 Schema Validation Tests
```
internal/validator/cascade_validator_test.go
internal/validator/cue_loader_test.go
```

**Key Test Scenarios**:
- CUE module loading with third-party dependencies
- Cascade validation with fallback chains
- Schema resolution and metadata extraction

#### 3.2 Supporting Infrastructure Tests
```
internal/errors/errors_test.go
internal/git/git_test.go
internal/cue/processor_test.go
```

### Phase 4: Integration & E2E Testing (Week 5)
**Estimated Effort**: 8-12 hours

#### 4.1 Integration Test Suite
```
test/integration/
├── import_workflow_test.go
├── streaming_import_test.go
├── schema_validation_test.go
└── migration_test.go
```

#### 4.2 Test Infrastructure
```
test/testutil/
├── fixtures.go          # Test data and schemas
├── temp_dirs.go         # Temporary directory management
├── mock_services.go     # Mock implementations
└── assertions.go        # Custom test assertions
```

## Specific Test Files to Create

### High Priority Test Files

#### `internal/importer/importer_test.go`
```go
// Test scenarios:
// - ImportFile with various formats (JSON, YAML, CSV, NDJSON)
// - Schema assignment with different confidence levels
// - Error handling for invalid files
// - Streaming vs non-streaming import modes
// - Collection processing with item extraction
```

#### `internal/database/catalog_test.go`
```go
// Test scenarios:
// - CRUD operations (AddEntry, GetEntry, QueryEntries)
// - Complex queries with filters and sorting
// - Collection and item relationships
// - Database initialization and schema creation
// - Concurrent access patterns
```

#### `cmd/import_test.go`
```go
// Test scenarios:
// - Command execution with various file types
// - Flag parsing and validation
// - Error handling and user feedback
// - Integration with importer service
// - Temporary directory cleanup
```

#### `internal/validator/cascade_validator_test.go`
```go
// Test scenarios:
// - Schema loading from CUE modules
// - Cascade validation with fallback chains
// - Third-party module integration
// - Validation error reporting
// - Schema metadata extraction
```

### Mock and Fixture Requirements

#### Test Data Fixtures
```
test/fixtures/
├── data/
│   ├── valid_json.json
│   ├── invalid_json.json
│   ├── ndjson_collection.json
│   ├── kubernetes_pods.yaml
│   ├── aws_ec2_instances.json
│   └── large_dataset.json
├── schemas/
│   ├── aws/
│   ├── k8s/
│   └── collections/
└── configs/
    ├── valid_config.yaml
    ├── invalid_config.yaml
    └── minimal_config.yaml
```

#### Mock Services
```go
// MockRuleEngine for testing schema assignment
// MockProgressReporter for testing streaming
// MockFileSystem for testing file operations
// MockGitRepository for testing git operations
```

## Quality Standards & Best Practices

### Test Structure
```go
func TestFunctionName(t *testing.T) {
    // Arrange
    setup := createTestSetup(t)
    defer setup.cleanup()
    
    // Act
    result, err := functionUnderTest(input)
    
    // Assert
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Test Categories
- **Unit Tests**: Fast (<10ms), no external dependencies
- **Integration Tests**: Medium speed (<1s), real file system
- **E2E Tests**: Slower (<10s), full workflow validation

### Error Testing
```go
func TestErrorScenarios(t *testing.T) {
    tests := []struct {
        name        string
        input       interface{}
        expectedErr string
    }{
        {"invalid format", invalidData, "unsupported format"},
        {"missing file", "", "file not found"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := functionUnderTest(tt.input)
            assert.Contains(t, err.Error(), tt.expectedErr)
        })
    }
}
```

### Performance Testing
```go
func BenchmarkImportLargeFile(b *testing.B) {
    setup := createBenchmarkSetup(b)
    defer setup.cleanup()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = importer.ImportFile(largeTestFile)
    }
}
```

## Dependencies and Infrastructure

### Testing Dependencies
```go
// Add to go.mod:
github.com/stretchr/testify v1.8.4
github.com/golang/mock v1.6.0
github.com/testcontainers/testcontainers-go v0.20.1
```

### CI/CD Integration
```yaml
# .github/workflows/test.yml
- name: Run Tests
  run: |
    go test -v -race -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    
- name: Coverage Check
  run: |
    coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    if (( $(echo "$coverage < 70" | bc -l) )); then
      echo "Coverage $coverage% is below 70% threshold"
      exit 1
    fi
```

## Success Metrics

### Coverage Targets
- **Overall Project**: 70%+ coverage
- **Core Business Logic**: 85%+ coverage
- **CLI Commands**: 60%+ coverage
- **Critical Paths**: 95%+ coverage

### Quality Metrics
- **Test Execution Time**: <30s for full suite
- **Flaky Test Rate**: <1%
- **Test Maintenance**: Tests should be self-documenting
- **Error Coverage**: All error paths tested

## Implementation Timeline

### Week 1-2: Foundation (Priority 1)
- [ ] `internal/importer/*_test.go` - Core import logic
- [ ] `internal/database/*_test.go` - Database operations
- [ ] `internal/config/config_test.go` - Configuration system

### Week 3: CLI Testing (Priority 2)
- [ ] `cmd/init_test.go` - Initialization command
- [ ] `cmd/import_test.go` - Import command
- [ ] `cmd/list_test.go` - List command
- [ ] `cmd/show_test.go` - Show command

### Week 4: Advanced Features (Priority 3)
- [ ] `internal/validator/*_test.go` - Schema validation
- [ ] `internal/errors/errors_test.go` - Error handling
- [ ] `internal/git/git_test.go` - Git operations

### Week 5: Integration & Polish
- [ ] `test/integration/*_test.go` - End-to-end workflows
- [ ] `test/testutil/*` - Test utilities and fixtures
- [ ] CI/CD integration and coverage reporting

---

*This comprehensive testing strategy will significantly improve PUDL's reliability, maintainability, and user confidence while providing excellent documentation of expected behavior through tests.*

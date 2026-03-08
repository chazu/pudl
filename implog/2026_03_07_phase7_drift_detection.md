# Phase 7: Drift Detection

**Date:** 2026-03-07

## Summary

Implemented drift detection: comparing a definition's declared state (socket bindings from the CUE definition file) against live state (the output of executing a method, stored as an artifact). Reports added/removed/changed fields with dot-notation paths.

## Design Decisions

- **JSON deep diff** instead of CUE subsume — declared state is `map[string]string` (socket bindings) and live state is method output JSON. Field-level diff is more actionable than structural unification.
- **Last artifact reuse** — Uses `db.GetLatestArtifact(definition, method)` to get live state. `--refresh` flag optionally re-executes the method first via the `StepExecutor` interface.
- **Report storage** — JSON files in `.pudl/data/.drift/<definition>/<timestamp>.json`, mirroring the workflow manifest pattern.
- **Configurable method** — `--method` flag specifies which method's artifact to compare. Default: first action method found on the model (preferring "list" or "describe").

## Public API

### `internal/drift` Package

```go
// Types
type DriftResult struct { Definition, Method, Status string; Timestamp time.Time; DeclaredKeys, LiveState map[string]interface{}; Differences []FieldDiff }
type FieldDiff struct { Path, Type string; Declared, Live interface{} }

// Comparator
func Compare(declared, live map[string]interface{}) []FieldDiff

// Checker
type Checker struct { ... }
func NewChecker(defDisc, modelDisc, db, executor, dataPath) *Checker
func (c *Checker) Check(ctx, CheckOptions) (*DriftResult, error)

// Report Store
type ReportStore struct { ... }
func NewReportStore(dataPath string) *ReportStore
func (s *ReportStore) Save(result *DriftResult) error
func (s *ReportStore) List(definitionName string) ([]string, error)
func (s *ReportStore) Get(definitionName, id string) (*DriftResult, error)
func (s *ReportStore) GetLatest(definitionName string) (*DriftResult, error)
```

### CLI Commands

- `pudl drift check <definition> [--method M] [--refresh] [--tag k=v]` — Run drift detection
- `pudl drift check --all` — Check all definitions
- `pudl drift report <definition>` — Show latest saved report

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `internal/drift/drift.go` | 119 | Types + recursive JSON deep diff comparator |
| `internal/drift/checker.go` | 144 | Drift checker with refresh support |
| `internal/drift/report.go` | 105 | Report storage (save/list/get/getLatest) |
| `internal/drift/drift_test.go` | 149 | Comparator tests (identical, changed, added, removed, nested, numeric coercion) |
| `internal/drift/checker_test.go` | 188 | Checker tests with mock executor and temp DB |
| `internal/drift/report_test.go` | 106 | Report round-trip tests |
| `cmd/drift.go` | 22 | Parent `drift` command |
| `cmd/drift_check.go` | 169 | `drift check` command |
| `cmd/drift_report.go` | 62 | `drift report` command |

## Test Results

All 16 drift tests pass. Full suite (`go test ./...`) remains green.

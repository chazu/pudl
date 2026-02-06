# 2026-02-06: Codebase Cleanup

## Summary

Major cleanup of the PUDL codebase: removed dead code, split oversized files, and updated documentation to reflect reality.

## Changes

### 1. Moved ValidationService to internal/validator/
- Created `internal/validator/validation_service.go`
- Moved `ValidationService`, `NewValidationService`, and related methods from `internal/review/`
- Renamed types to avoid conflicts: `ServiceValidationResult`, `ServiceCascadeAttempt`
- Updated `cmd/validate.go` imports accordingly

### 2. Removed internal/review/ (entire package)
- Deleted 7 files: `session.go`, `interface.go`, `fetcher.go`, `catalog_updater.go`, `schema_creator.go`, `validation.go`, `schema.cue.tmpl`
- Removed `schemaReviewCmd` and all review-related functions from schema command
- Inlined `CatalogUpdater.UpdateSingleEntry()` directly in reinfer command
- Updated validate command suggestion text to reference `pudl schema reinfer --all`

### 3. Split cmd/schema.go into focused files
- Original: ~1900 lines in a single file
- Result: 9 files, each under 300 lines:
  - `cmd/schema.go` - Root schemaCmd + shared vars (52 lines)
  - `cmd/schema_list.go` - List schemas
  - `cmd/schema_add.go` - Add schema files
  - `cmd/schema_git.go` - Git operations (status, commit, log)
  - `cmd/schema_new.go` - Generate schemas from data
  - `cmd/schema_edit.go` - Edit schemas
  - `cmd/schema_show.go` - Show schema contents
  - `cmd/schema_reinfer.go` - Re-infer schema assignments
  - `cmd/schema_migrate.go` - Migrate schema names
- Each file has its own `init()` for command/flag registration

### 4. Removed cmd/git.go
- Deleted the `pudl git` / `pudl git cd` command

### 5. Fixed root command description
- Removed "Automatic schema inference using embedded Lisp rules"
- Replaced with "Automatic CUE-based schema inference with cascade validation"

### 6. Updated docs/VISION.md
- Separated "What Exists Today" from "Future Vision"
- Removed all Zygomys/Lisp references
- Removed Bubble Tea TUI references
- Organized future work into phased roadmap

### 7. Updated docs/plan.md
- Marked completed items
- Removed stale Lisp references
- Added phased future roadmap from project review
- Listed remaining cut candidates
- Updated core packages reference

## Public API Changes
- `review.ValidationService` -> `validator.ValidationService`
- `review.ValidationResult` -> `validator.ServiceValidationResult`
- `review.CascadeAttempt` -> `validator.ServiceCascadeAttempt`
- Removed: `pudl schema review` command
- Removed: `pudl git` command

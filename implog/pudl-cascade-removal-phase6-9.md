# Cascade Removal & Schema Simplification - Phases 6-9

## Summary

Removed the cascade priority system, replacing it with native CUE unification. Simplified the definition package by renaming ModelRef to SchemaRef and broadening pattern matching. Cleaned up bootstrap schemas and all test/metadata references.

## Phase 6: Bootstrap Schema Cleanup

- Removed `cascade_priority`, `cascade_fallback`, `compliance_level` from `pudl/core/core.cue` (_pudl metadata in #Item and #Collection)
- Deleted `internal/importer/bootstrap/pudl/model/` (model schema + examples)
- Deleted `internal/importer/bootstrap/definitions/` (example definitions referencing models)

## Phase 7: Replace Cascade with CUE Unification

**Validator changes:**
- `SchemaMetadata`: removed `CascadePriority`, `CascadeFallback`, `ComplianceLevel` fields
- `ValidationResult`: removed `CascadeLevel`, `ComplianceStatus` fields. `SetFinalAssignment` now takes only schema name + fallback reason. Added `Valid` field (binary pass/fail).
- `CascadeValidator.ValidateWithCascade`: replaced `getCascadeChain` (priority-based) with `buildValidationChain` (walks base_schema chain up to catchall). Removed `determineCascadeLevel` and `determineFallbackReason`.
- `GetSchemasByResourceType`: replaced priority sort with alphabetical sort.
- `ValidationService`: removed `CascadeLevel`, `ComplianceStatus` from `ServiceValidationResult`. Removed `GetDetailedValidationReport` (replaced by `GetValidationSummary`). Removed `IsCompliant`/`IsNonCompliant`/`IsUnknown`/`GetSeverity` methods.

**Importer changes:**
- Removed `ComplianceStatus` and `CascadeLevel` from `SchemaInfo` struct
- Removed `getComplianceStatus()` and `getCascadeLevel()` helpers
- Removed compliance status display from `cmd/import.go`

**Typepattern changes:**
- Removed `CascadePriority` from `PudlMetadata` struct and all pattern defaults

**Schemagen changes:**
- Removed `cascade_priority` and `compliance_level` from generated CUE output

## Phase 8: Simplify Inference Graph

- Removed `priority` map from `InheritanceGraph`
- `GetMostSpecificFirst()` tiebreaker changed from cascade_priority to alphabetical
- `shouldSwap()` in heuristics uses alphabetical tiebreaker instead of priority
- Removed `GetPriority()` method

## Phase 9: Simplify Definition Package

- Renamed `ModelRef` to `SchemaRef` in `DefinitionInfo`
- Broadened `modelUnifyPattern` regex to `schemaUnifyPattern` — matches any `pkg.#Name &` pattern, not just `#*Model`
- Renamed `_model:` marker to `_schema:` marker
- Updated `cmd/definition.go`, `cmd/definition_list.go`, `cmd/definition_show.go` — renamed flag `--model` to `--schema`, updated display text
- Rewrote `definition_test.go` with self-contained test fixtures (no longer depends on deleted bootstrap definitions)

## Files Modified (across all 4 phases)

~40 files touched across `internal/validator/`, `internal/inference/`, `internal/importer/`, `internal/definition/`, `internal/typepattern/`, `internal/schemagen/`, `internal/init/`, `cmd/`, `test/`, and bootstrap CUE files.

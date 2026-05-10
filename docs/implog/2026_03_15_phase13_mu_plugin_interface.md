# Phase 13: Mu Plugin Interface

**Date:** 2026-03-15

## Summary

Added `pudl export-actions` command and `internal/mubridge` package to bridge pudl's drift detection to mu's execution engine. Drift reports are converted into mu-compatible JSON plan responses containing action specs.

## Files Created

- `internal/mubridge/export.go` — Types and conversion logic
- `internal/mubridge/export_test.go` — Unit tests (3 test cases)
- `cmd/export_actions.go` — CLI command definition

## Files Modified

- `internal/drift/report.go` — Added `ReportStore.ListDefinitions()` method
- `docs/plan.md` — Added Phase 13 section and package entry

## Public API

### Package `mubridge`

- `ActionSpec` — Struct matching mu's plugin protocol format (ID, Command, Inputs, Outputs, DependsOn, Env, Network)
- `PlanResponse` — Struct matching mu's plan response format (Actions, Outputs, Error)
- `ExportFromDriftReport(report *drift.DriftResult) *PlanResponse` — Converts a drift result into a plan response with one action per field difference

### Package `drift` (addition)

- `(*ReportStore).ListDefinitions() ([]string, error)` — Returns names of all definitions that have saved drift reports

### CLI

- `pudl export-actions --definition <name>` — Export actions for a single definition's latest drift report
- `pudl export-actions --all` — Export actions for all definitions with drift reports (merged into one plan response)

## Design Decisions

- Each `FieldDiff` maps to one `ActionSpec` with placeholder `echo` commands describing the drift
- The `--all` flag merges all definitions into a single `PlanResponse`, namespacing outputs as `<definition>.<key>`
- Output is a single JSON object (not NDJSON) per mu's plugin protocol (one plan response per invocation)

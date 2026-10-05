# Phase 14: Directory Structure Validation in Doctor Command

**Date:** 2026-03-15

## Summary

Added a `CheckDirectoryStructure()` health check to the `pudl doctor` command that validates the `~/.pudl/` directory structure, inspired by defn's `manifest/manifest.cue` `close({})` pattern for exhaustive validation.

## What Was Built

### Public API

- `doctor.CheckDirectoryStructure() *CheckResult` — validates the full `~/.pudl/` directory structure

### Checks Performed

1. `~/.pudl/data/` exists with `raw/` and `sqlite/` subdirectories
2. `~/.pudl/data/sqlite/catalog.db` exists if raw data has been imported
3. `~/.pudl/schema/` exists with `cue.mod/` (CUE module)
4. `~/.pudl/schema/pudl/core/` exists (bootstrap schemas)
5. No unexpected top-level directories in `~/.pudl/` (warns about anything not `data`, `schema`, or `config.yaml`)
6. Raw data follows `YYYY/MM/DD` date hierarchy

### Internal Helpers

- `validateRawDataHierarchy(rawDir string) []string` — walks the raw data directory and returns paths violating the YYYY/MM/DD pattern
- `isNumericDir(name string, expectedLen int) bool` — validates numeric directory names

### Registration

The check is registered in `cmd/doctor.go` as "Directory Structure" in the checks slice, positioned before "Orphaned Files".

## Files Changed

- `internal/doctor/checks.go` — added `CheckDirectoryStructure()`, `validateRawDataHierarchy()`, `isNumericDir()`
- `cmd/doctor.go` — registered new check, updated Long description

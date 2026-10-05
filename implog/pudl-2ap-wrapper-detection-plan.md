# pudl-2ap: Implementation Plan — Collection Wrapper Detection + Unwrap

**Date:** 2026-02-18
**Task:** pudl-2ap
**Type:** Implementation plan (no code written)

## Summary

Created a detailed implementation plan for Option A (import-time collection wrapper detection and unwrap) based on research from pudl-yqt and pudl-mxk.

## Plan Location

`docs/plan.md` — new section "Implementation Plan: Collection Wrapper Detection + Unwrap (Option A)"

## Task Breakdown

1. **Task 1:** `internal/importer/wrapper.go` — Detection logic with scoring algorithm (~200 lines)
2. **Task 2:** `internal/importer/wrapper_test.go` — Unit tests with 8+ positive, 7+ negative, 4 edge cases (~250 lines)
3. **Task 3:** Integration into `importer.go` import path (~30 lines added)
4. **Task 4:** Fix CollectionType hint propagation in `assignItemSchema()` (~2 lines)
5. **Task 5:** Integration tests in `importer_test.go` (~80 lines)

## Key Design Decisions

- Scoring heuristics with threshold ≥ 0.50 (requires ≥2 signals)
- Case-insensitive key matching via `strings.EqualFold`
- Reuses existing `createCollectionEntry` / `createCollectionItems` infrastructure
- No nested wrapper detection (out of scope)
- No user-extensible key lists yet (hardcoded, extensible later)

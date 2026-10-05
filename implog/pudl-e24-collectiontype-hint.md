# pudl-e24: Fix CollectionType hint propagation in assignItemSchema

**Date:** 2026-02-18

## Summary

Added `CollectionType: "item"` to the `InferenceHints` struct in `assignItemSchema()` so that the collection/item filtering logic in `heuristics.go:71-83` activates when inferring schemas for individual collection items.

## Changes

- `internal/importer/importer.go`: Added `CollectionType: "item"` to the `InferenceHints` struct passed to `i.inferrer.Infer()` in `assignItemSchema()` (line 844).

## Impact

The heuristics system can now filter schema candidates based on whether data is an item (vs collection), improving schema inference accuracy for collection items.

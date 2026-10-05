# pudl-yqt: Research — Built-in Schema for API Collection Wrapper Responses

**Date**: 2026-02-18
**Type**: Research
**Status**: Complete

## Summary

Researched how to detect and handle API responses that wrap collections in an
object (e.g., `{"Items": [...], "Count": 42}`). Currently pudl only recognizes
NDJSON as collections; wrapped JSON collections are treated as single objects.

## Key Findings

1. **Current gap**: No wrapper detection exists. The `CollectionType` inference
   hint is defined but never passed during import, so collection/item schema
   filtering never activates.

2. **Common wrapper patterns**: Surveyed key names (`items`, `data`, `results`,
   `records`, etc.), pagination signals (`next_token`, `total`, `has_more`, etc.),
   and structural patterns across major APIs (AWS, Stripe, Google, Elasticsearch).

3. **False positive risk**: Must distinguish wrapper objects from resources that
   happen to have array fields (e.g., `tags`, `labels`). Proposed a scoring
   algorithm combining key name matching, pagination signal detection, array
   homogeneity, and negative signals (known attribute names, multiple arrays).

## Recommendation

**Detection + Unwrap at Import Time**: Add a `DetectCollectionWrapper()` function
in a new `internal/importer/wrapper.go` that scores objects using the heuristics.
When a wrapper is detected, extract items and route to the existing collection
import pipeline. Also fix `CollectionType` hint propagation so inference correctly
filters collection vs item schemas.

## Deliverable

Full research document: `.ai/research/pudl-yqt-collection-wrapper-schema.md`

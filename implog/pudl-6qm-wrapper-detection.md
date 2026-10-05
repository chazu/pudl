# pudl-6qm: Create wrapper.go — collection wrapper detection logic

## Summary

Created `internal/importer/wrapper.go` with the `DetectCollectionWrapper` scoring algorithm. This is a pure function with no side effects that analyzes a `map[string]interface{}` and determines if it represents an API collection wrapper response (e.g., `{"items": [...], "count": 2}`).

## Public API

- `WrapperDetection` struct — holds detection result: `ArrayKey`, `Items`, `WrapperMeta`, `Score`, `Signals`
- `DetectCollectionWrapper(data map[string]interface{}) *WrapperDetection` — main detection entry point, returns nil if not a wrapper or below threshold (0.50)
- `KnownWrapperKeys` — exported var for known collection array field names
- `KnownAttributeKeys` — exported var for known attribute array field names (negative signal)
- `PaginationKeys` — exported var for known pagination/metadata field names

## Scoring Signals

| Signal | Score Delta |
|--------|-------------|
| Known wrapper key | +0.35 |
| Pagination siblings | +0.25 |
| Count matches length | +0.20 |
| Homogeneous elements (≥80%) | +0.15 |
| Few top-level keys (≤5) | +0.05 |
| Dominant array | +0.05 |
| Known attribute key | -0.30 |
| Multiple similar arrays | -0.40 |
| Many scalar fields (>6) | -0.15 |

## Internal Helpers

- `scoreCandidate` — scores a single array field candidate
- `hasPaginationSiblings` — checks for pagination keys among siblings
- `countMatchesArrayLength` — checks if any numeric sibling equals array length
- `isHomogeneous` — checks if ≥threshold of elements share same key set
- `isDominantArray` — checks if array is largest field by element count
- `hasMultipleSimilarArrays` — checks for ≥2 other object arrays
- `hasManyScalarFields` — checks for >6 non-pagination scalar fields
- `extractWrapperMeta` — returns all fields except the array key
- `containsFold` — case-insensitive string slice membership
- `sortStrings` — simple insertion sort for key signatures

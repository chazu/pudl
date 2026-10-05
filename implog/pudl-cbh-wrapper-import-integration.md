# pudl-cbh: Integrate wrapper detection into import path

## Summary

Added wrapper detection call to `ImportFile()` in `internal/importer/importer.go` and created the `importWrappedCollection()` method. When a JSON object is detected as a collection wrapper (e.g., `{"items": [...], "count": 2}`), the embedded array is extracted and imported as a collection of individual items — reusing the existing `createCollectionEntry`/`createCollectionItems` infrastructure.

## Changes

### `internal/importer/importer.go`

1. **Wrapper detection call** (after `analyzeDataStreaming`): If the parsed data is a `map[string]interface{}`, call `DetectCollectionWrapper()`. If a wrapper is detected, route to `importWrappedCollection()` and return early.

2. **`importWrappedCollection()` method**: New method that:
   - Copies the original file to raw storage
   - Calls `createCollectionEntry()` with extracted items and item count
   - Calls `createCollectionItems()` to create individual item catalog entries
   - Returns the collection `ImportResult`

## Public API

No new public API — the wrapper detection is transparently integrated into the existing `ImportFile()` flow. The `DetectCollectionWrapper()` function (from `wrapper.go`) and `WrapperDetection` struct are already public from the prior task.

## Lines added

~30 lines added to `importer.go`.

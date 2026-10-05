# Streaming Parser Cross-Chunk Reassembly Fix

**Date:** 2026-02-09

## Summary

Fixed the streaming parser to properly handle large JSON/YAML/CSV files by implementing cross-chunk reassembly logic. Also fixed CUE schema generation issues.

## Changes

### 1. CUE Field Name Quoting (internal/schemagen/generator.go)
- Added `needsQuoting()` function to detect field names requiring quotes (hyphens, dots, slashes, etc.)
- Added `formatFieldName()` function to apply quoting when needed
- Updated `writeFields()` to use proper quoting for all field names
- Added `ValidateCUEContent()` for pre-write schema validation
- Fixed cascade fallback path from `"core.#Item"` to `"pudl/core.#Item"`

### 2. Streaming Parser Interface Updates (internal/streaming/interfaces.go)
- Added `Finalize() (*ProcessedChunk, error)` to ChunkProcessor interface
- Added `Reset()` to ChunkProcessor interface
- Added `GetBufferSize() int` to ChunkProcessor interface

### 3. JSON Processor Fixes (internal/streaming/json_processor.go)
- Implemented `Finalize()` for flushing buffered data at EOF
- Implemented `Reset()` for processor reuse
- Implemented `GetBufferSize()` for buffer inspection
- Fixed NDJSON detection to only count lines starting at column 0 (no leading whitespace)
  - Previous logic falsely detected formatted single JSON objects as NDJSON

### 4. YAML Processor Fixes (internal/streaming/yaml_processor.go)
- Implemented `Finalize()`, `Reset()`, `GetBufferSize()` methods

### 5. CSV Processor Fixes (internal/streaming/csv_processor.go)
- Implemented `Finalize()`, `Reset()`, `GetBufferSize()` methods

### 6. Parser Core Fixes (internal/streaming/parser.go)
- Fixed CDC EOF handling to process final chunk when `io.EOF` is returned with data
- Added processor state persistence across chunks (reuse same processor instance)
- Added format detection state (`streamFormat`, `currentProcessor`, `formatDetected`)
- Call `Finalize()` on processor at end of stream

### 7. Generic Processor Updates (internal/streaming/processors.go)
- Added `format: "unknown"` field to text and binary chunk objects for consistency

## Tests Added (internal/streaming/parser_test.go)
- `TestLargeJSONArrayCrossChunkReassembly` - 100 objects in JSON array
- `TestLargeNDJSONCrossChunkReassembly` - 200 NDJSON lines
- `TestLargeYAMLCrossChunkReassembly` - 50 YAML documents
- `TestLargeCSVCrossChunkReassembly` - 500 CSV rows
- `TestVeryLargeJSONFile` - 1000 objects (~1MB)
- `TestProcessorReuseAcrossChunks` - Verifies processor reuse

## Tests Added (internal/schemagen/generator_test.go)
- `TestNeedsQuoting` - 21 test cases for identifier detection
- `TestFormatFieldName` - 7 test cases for quoting behavior
- `TestValidateCUEContent` - 4 test cases for CUE validation
- `TestSchemaValidationError` - 2 test cases for error type

## Bug Fixes

1. **CDC EOF Handling**: The go-cdc-chunkers library returns the last chunk AND `io.EOF` together. Code was breaking on EOF without processing the final chunk.

2. **NDJSON False Positive**: Formatted single JSON objects with nested content (like `"Tags": [`) were falsely detected as NDJSON.

3. **CUE Field Quoting**: Field names with special characters weren't quoted, producing invalid CUE syntax.

## Public API

No new public API. Internal improvements to:
- `streaming.ChunkProcessor` interface (3 new methods)
- `streaming.DefaultStreamingParser` (processor state management)
- `schemagen.ValidateCUEContent()` (new validation function)

## Verification

- All 51 tests pass
- 1.8MB configmaps.json imports successfully
- 3.8MB pods.json imports successfully


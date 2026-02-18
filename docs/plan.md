# PUDL - Development Plan

## Project Overview

PUDL (Personal Unified Data Lake) is a CLI tool for SRE/platform engineers to manage and analyze cloud infrastructure data. It creates a personal data lake for cloud resources, Kubernetes objects, logs, and metrics using CUE-based schema validation.

## Current State

### Core Functionality (Implemented)
- **Data Import** - JSON, YAML, CSV, NDJSON with automatic format detection
- **Collection Support** - Collections split into individual items with parent references
- **SQLite Catalog** - Query, filter, pagination, and provenance tracking
- **CUE Schema Management** - Loading, validation, git version control, schema lifecycle
- **CUE-based Schema Inference** - Automatic detection using heuristics + CUE unification
- **Cascade Validation** - Multi-level schema matching (specific -> fallback -> catchall)
- **Schema Generation** - `pudl schema new` generates CUE schemas from imported data
- **Schema Name Normalization** - Canonical format for consistent schema references
- **Bootstrap Schemas** - Base schemas (catchall, collections) embedded and copied on init
- **Export** - Multi-format output support
- **Delete** - Remove catalog entries
- **Doctor** - Health check utility

### CLI Commands
- `pudl init` - Initialize the data lake
- `pudl import` - Import data files
- `pudl list` - Query and filter catalog entries
- `pudl export` - Export data in various formats
- `pudl delete` - Remove catalog entries
- `pudl validate` - Validate data against schemas
- `pudl schema list` - List available schemas by package
- `pudl schema add` - Add a new schema file
- `pudl schema new` - Generate schema from imported data
- `pudl schema show` - Display schema contents
- `pudl schema edit` - Open schema in editor
- `pudl schema reinfer` - Re-run schema inference on existing entries
- `pudl schema migrate` - Migrate schema names to canonical format
- `pudl schema status` - Show uncommitted schema changes
- `pudl schema commit` - Commit schema changes
- `pudl schema log` - Show schema commit history
- `pudl doctor` - Health check utility
- `pudl migrate identity` - Backfill resource identity tracking for existing entries

### Schema Infrastructure
- `internal/importer/bootstrap/` - Embedded bootstrap CUE schemas
- `internal/schema/manager.go` - Schema loading and management
- `internal/validator/` - CUE validation, cascade validation, and validation service
- `internal/inference/` - CUE-based schema inference with heuristics
- `internal/schemaname/` - Schema name normalization
- `internal/schemagen/` - Schema generation from data

### User Repository (`~/.pudl/`)
```
~/.pudl/
├── config.yaml           # Configuration file
├── data/                  # Imported data files
├── schema/
│   ├── cue.mod/
│   │   └── module.cue    # CUE module
│   ├── pudl/
│   │   ├── core/         # Core schemas (catchall)
│   │   ├── collections/  # Collection schemas
│   │   └── ...           # User-created schema packages
│   └── ...
└── catalog.db            # SQLite catalog
```

## Completed Work

### 2026-01-29 Cleanup
- [x] Removed `internal/importer/cue.mod/` - Was incorrectly creating CUE module in project repo
- [x] Removed `internal/importer/pudl/` - Duplicate of bootstrap/pudl directory
- [x] Consolidated CUE module creation
- [x] Simplified `detectOrigin()` - Uses filename only; schema matching handled by CUE
- [x] Updated tests to reflect simplified detection

### 2026-02-06 Codebase Cleanup
- [x] Removed `internal/review/` - Interactive review workflow removed (untested, unused)
- [x] Moved `ValidationService` from `internal/review/` to `internal/validator/`
- [x] Removed `cmd/git.go` - Redundant `pudl git cd` command
- [x] Split `cmd/schema.go` (~1900 lines) into focused files (~9 files, each under 300 lines)
- [x] Fixed root command description (removed stale Lisp reference)
- [x] Updated `docs/VISION.md` to separate existing features from aspirational

### 2026-02-06 Resource Identity Tracking
- [x] Created `internal/identity/` package — field extraction + resource ID computation
- [x] Database schema evolution — new columns, migrations, identity query methods
- [x] Import flow integration — content hash dedup, identity extraction, versioning
- [x] Reinfer integration — recompute identity when schema changes
- [x] Lister and CLI updates — version display, identity fields in verbose mode
- [x] Backfill command — `pudl migrate identity` for pre-existing entries
- [x] Unit + integration tests (30 new tests)

### 2026-02-09 Streaming Parser Fix & Schema Generation
- [x] Fixed CUE field name quoting — Fields with special characters now properly quoted
- [x] Added pre-write schema validation — `ValidateCUEContent()` validates before writing
- [x] Fixed cascade fallback path format — Uses `"pudl/core.#Item"` not `"core.#Item"`
- [x] Fixed CDC EOF handling — Final chunk now processed when `io.EOF` returned with data
- [x] Fixed NDJSON false positive detection — Only count lines at column 0
- [x] Implemented cross-chunk reassembly — Processor state persisted across chunks
- [x] Added `Finalize()`, `Reset()`, `GetBufferSize()` to ChunkProcessor interface
- [x] Large file tests — 6 new tests for cross-chunk reassembly up to 1MB

### 2026-02-18 Collection Wrapper Detection Research
- [x] pudl-yqt: Researched built-in schema for API collection wrapper responses
- [x] pudl-mxk: Deep dive into Option B (CUE schema-based detection) — concluded CUE type system cannot express the structural constraint; Option A (import-time unwrap) is recommended

### Design Decisions Made
- **No Lisp/Zygomys rules** - Schema inference uses CUE-based detection, not a Lisp rules engine
- **No interactive review TUI** - Review workflow removed; `pudl schema reinfer` handles batch re-inference
- **Schema inference via CUE** - Heuristics + CUE unification for automatic schema detection
- **Schema name normalization** - Canonical `<package>.<#Definition>` format
- **Resource identity** - Stable `resource_id` from schema + identity fields; catchall uses content hash
- **Content hash dedup** - Universal dedup gate: if hash matches, skip regardless of schema
- **Collections are provenance** - Resources own identity independent of collection

## Implementation Plan: Collection Wrapper Detection + Unwrap (Option A)

Based on research from pudl-yqt and pudl-mxk, this plan implements import-time detection and unwrapping of API collection wrapper responses (e.g., `{"items": [...], "count": 2}`). When a JSON object is detected as a collection wrapper, the embedded array is extracted and imported as a collection of individual items — the same way NDJSON files are handled today.

### Architecture Overview

```
ImportFile()
  ├── detectFormat()        ← existing
  ├── analyzeDataStreaming() ← existing
  ├── detectCollectionWrapper(data)  ← NEW: wrapper detection
  │     └── if wrapper detected:
  │           ├── extract items array
  │           ├── route to importWrappedCollection() ← NEW
  │           │     ├── createCollectionEntry()      ← reuse existing
  │           │     └── createCollectionItems()      ← reuse existing (with CollectionType hint)
  │           └── return early
  └── ... normal single-object import path (unchanged)
```

### Task Breakdown

#### Task 1: `internal/importer/wrapper.go` — Detection Logic (no dependencies)

Create a new file with the wrapper detection scoring algorithm. This is the core of the feature — a pure function with no side effects that analyzes a `map[string]interface{}` and decides if it's a collection wrapper.

**Public API:**
```go
// WrapperDetection holds the result of collection wrapper detection.
type WrapperDetection struct {
    ArrayKey       string                 // Key name of the array field (e.g., "items")
    Items          []interface{}          // Extracted array elements
    WrapperMeta    map[string]interface{} // Non-array sibling fields (pagination, count, etc.)
    Score          float64                // Confidence score (0.0–1.0)
    Signals        []string               // Human-readable list of signals that contributed
}

// DetectCollectionWrapper analyzes a JSON object and determines if it's a
// collection wrapper (e.g., {"items": [...], "count": 2}).
// Returns nil if the data is not a wrapper or the score is below threshold.
func DetectCollectionWrapper(data map[string]interface{}) *WrapperDetection
```

**Scoring algorithm** (from pudl-yqt research, refined by pudl-mxk):

For each top-level field where value is `[]interface{}` with len ≥ 1 and first element is `map[string]interface{}`:

| Signal | Condition | Score Delta |
|--------|-----------|-------------|
| Known wrapper key | `arrayKey ∈ KNOWN_WRAPPER_KEYS` | +0.35 |
| Pagination siblings | Any sibling key ∈ `PAGINATION_KEYS` | +0.25 |
| Count matches length | Sibling numeric field == array length | +0.20 |
| Homogeneous elements | ≥80% of elements share same top-level keys | +0.15 |
| Few top-level keys | Total keys ≤ 5 | +0.05 |
| Dominant array | Array is the largest field by estimated size | +0.05 |
| Known attribute key | `arrayKey ∈ KNOWN_ATTRIBUTE_KEYS` | −0.30 |
| Multiple similar arrays | ≥2 array fields of similar size | −0.40 |
| Many scalar fields | >6 non-pagination scalar fields | −0.15 |

Threshold: score ≥ 0.50.

If multiple array fields are candidates, the one with the highest score wins.

**Known key lists** (exported as package-level vars for testability):

- `KnownWrapperKeys`: `items`, `data`, `results`, `records`, `entries`, `objects`, `resources`, `hits`, `values`, `elements`, `list`, `rows`, `nodes`, `edges` (case-insensitive via `strings.EqualFold`)
- `KnownAttributeKeys`: `tags`, `labels`, `permissions`, `roles`, `addresses`, `emails`, `groups`, `scopes`, `features`, `capabilities`, `attachments`, `dependencies`, `headers`, `cookies`, `args`, `arguments`, `env`, `environment`, `ports`, `volumes`, `rules`, `conditions`
- `PaginationKeys`: `next_token`, `nextToken`, `NextToken`, `cursor`, `next_cursor`, `nextCursor`, `continuation_token`, `ContinuationToken`, `marker`, `NextMarker`, `next_marker`, `page`, `page_size`, `pageSize`, `per_page`, `perPage`, `offset`, `limit`, `total`, `totalCount`, `total_count`, `count`, `Count`, `has_more`, `hasMore`, `HasMore`, `_links`, `links`, `next`, `previous`, `meta`, `metadata`, `_metadata`

**Internal helper functions:**
- `scoreCandidate(key string, arr []interface{}, allFields map[string]interface{}) float64` — scores a single array field candidate
- `hasPaginationSiblings(fields map[string]interface{}, excludeKey string) bool` — checks for pagination keys among siblings
- `countMatchesArrayLength(fields map[string]interface{}, arrLen int, excludeKey string) bool` — checks if any numeric sibling equals the array length
- `isHomogeneous(arr []interface{}, threshold float64) bool` — checks if ≥threshold of elements share the same key set
- `extractWrapperMeta(fields map[string]interface{}, arrayKey string) map[string]interface{}` — returns all fields except the array

**File size estimate:** ~200 lines.

#### Task 2: `internal/importer/wrapper_test.go` — Unit Tests (depends on Task 1)

Comprehensive tests for `DetectCollectionWrapper`:

**Positive cases (should detect as wrapper):**
1. Simple wrapper: `{"items": [{"id": 1}, {"id": 2}]}` — known key + homogeneous
2. Wrapper + count: `{"items": [{"id": 1}, {"id": 2}], "count": 2}` — known key + count match
3. Wrapper + pagination: `{"data": [{"name": "a"}], "next_token": "abc", "total": 1}` — known key + pagination
4. Envelope pattern: `{"data": [{"x": 1}], "meta": {"page": 1}, "links": {"next": "/p2"}}` — known key + pagination
5. Stripe-like: `{"data": [{"id": "ch_1"}], "has_more": true, "url": "/charges"}` — known key + pagination
6. AWS DynamoDB: `{"Items": [{"pk": {"S": "1"}}], "Count": 1, "ScannedCount": 1}` — case-insensitive key + count match
7. Large homogeneous array: 50+ items with identical keys — high homogeneity signal
8. Elasticsearch hits: `{"hits": [{"_id": "1", "_source": {}}], "total": {"value": 1}}` — known key

**Negative cases (should NOT detect as wrapper):**
1. Resource with tags: `{"name": "server-1", "tags": ["web", "prod"], "id": "srv-1"}` — known attribute key + primitive array
2. Resource with array field: `{"data": [{"x": 1}], "name": "config", "version": "1.0", "type": "widget", "author": "joe", "created": "2024", "updated": "2024", "id": "cfg-1"}` — too many scalar fields
3. Multiple arrays: `{"users": [{"id": 1}], "roles": [{"id": 1}], "groups": [{"id": 1}]}` — multiple similar arrays penalty
4. Primitive array: `{"items": [1, 2, 3], "count": 3}` — array of non-objects
5. Empty array: `{"items": [], "count": 0}` — no elements to analyze
6. Single object (not a map): `[{"id": 1}]` — not a map at all (function takes map)
7. Resource with nested array: `{"name": "menu", "items": [{"label": "File"}], "parent_id": 3, "type": "nav", "icon": "menu", "position": 1, "visible": true}` — many scalar fields

**Edge cases:**
1. Exactly at threshold (score = 0.50): should detect
2. Just below threshold (score = 0.49): should not detect
3. Single-element array with known key + pagination: should detect
4. Unknown key name but strong pagination + count signals: should detect (if score ≥ 0.50 from other signals)

**File size estimate:** ~250 lines.

#### Task 3: Integration into `importer.go` — Wrapper Detection in Import Path (depends on Task 1) ✅ DONE

Modify `ImportFile()` in `importer.go` to call `DetectCollectionWrapper` after `analyzeDataStreaming()` returns a single object (`map[string]interface{}`).

**Changes to `importer.go`:**

1. **After line 206** (after `analyzeDataStreaming` returns `data`), add wrapper detection:
   ```go
   // Check if the data is a collection wrapper (e.g., {"items": [...], "count": 2})
   if dataMap, ok := data.(map[string]interface{}); ok {
       if wrapper := DetectCollectionWrapper(dataMap); wrapper != nil {
           return i.importWrappedCollection(opts, wrapper, timestamp, timestampStr, origin, filename, rawDir, metadataDir, fileInfo)
       }
   }
   ```

2. **New method `importWrappedCollection`** — similar to `importNDJSONCollection` but takes a `*WrapperDetection` instead of parsing NDJSON:
   ```go
   func (i *Importer) importWrappedCollection(
       opts ImportOptions,
       wrapper *WrapperDetection,
       timestamp time.Time,
       timestampStr, origin, filename string,
       rawDir, metadataDir string,
       fileInfo os.FileInfo,
   ) (*ImportResult, error)
   ```

   This method:
   - Copies the original file to raw storage (same as current flow)
   - Calls `createCollectionEntry()` with `wrapper.Items` as data and `len(wrapper.Items)` as record count
   - Stores wrapper metadata (pagination info) in collection metadata — add `WrapperMeta` field to the collection metadata, or store in existing flexible fields
   - Calls `createCollectionItems()` with the collection ID and `wrapper.Items`
   - Returns the collection `ImportResult`

**What stays unchanged:**
- NDJSON detection and import path (handled before wrapper detection, at line 181)
- The `else` branch for manual schema / automatic inference for non-wrapper objects
- All existing collection infrastructure (`createCollectionEntry`, `createCollectionItems`, `createCollectionItem`)

**File size impact:** ~30 lines added to `importer.go` (the `importWrappedCollection` method + the detection call).

#### Task 4: Fix CollectionType Hint Propagation (depends on Task 1, can be done in parallel with Task 3) ✅ DONE

Currently, `assignItemSchema()` (line 844) and the inference call in `ImportFile()` (line 230) do not pass `CollectionType` in `InferenceHints`. This means the collection/item filtering in `heuristics.go:71-83` never activates.

**Changes:**

1. **`assignItemSchema()`** at line 844 — add `CollectionType: "item"` to hints:
   ```go
   result, err := i.inferrer.Infer(itemData, inference.InferenceHints{
       Format:         "json",
       CollectionType: "item",
   })
   ```

2. **`createCollectionEntry()`** — if inference is ever added here (currently hardcoded to `#Collection`), pass `CollectionType: "collection"`.

3. **`importWrappedCollection()`** — when calling `createCollectionItems`, the items will flow through `assignItemSchema` which will now have the `CollectionType: "item"` hint.

**File size impact:** ~2 lines changed in `importer.go`.

#### Task 5: Integration Test (depends on Tasks 2, 3, 4) ✅ DONE

Add integration tests to `internal/importer/importer_test.go` that test the full import flow with wrapper data.

**Test cases:**
1. Import a JSON file containing `{"items": [{"id": "a"}, {"id": "b"}], "count": 2}`:
   - Verify the import returns a collection result with `RecordCount: 2`
   - Verify catalog has a collection entry + 2 item entries
   - Verify items are assigned schemas with `CollectionType: "item"` hint
2. Import a JSON file containing a normal object (non-wrapper) — verify the existing single-object path is unchanged
3. Import an NDJSON file — verify it still goes through the NDJSON path (not the wrapper path)

**File size impact:** ~80 lines added to `importer_test.go`.

### Dependency Order

```
Task 1: wrapper.go (detection logic)
  ↓
Task 2: wrapper_test.go (unit tests)     Task 4: CollectionType hint fix
  ↓                                        ↓
Task 3: importer.go integration ←─────────┘
  ↓
Task 5: Integration tests
```

Tasks 1 is the foundation. Tasks 2 and 4 can be done in parallel. Task 3 depends on 1 and 4. Task 5 depends on everything.

### File-by-File Summary

| File | Action | Lines (est.) |
|------|--------|-------------|
| `internal/importer/wrapper.go` | **NEW** | ~200 |
| `internal/importer/wrapper_test.go` | **NEW** | ~250 |
| `internal/importer/importer.go` | **MODIFY** | +30 (new method + detection call) |
| `internal/importer/importer.go` | **MODIFY** | +2 (CollectionType hint in assignItemSchema) |
| `internal/importer/importer_test.go` | **MODIFY** | +80 (integration tests) |

### Design Decisions

1. **Case-insensitive key matching**: Use `strings.EqualFold` for known key lists to handle `Items`, `ITEMS`, `items` etc.
2. **Threshold at 0.50**: Conservative enough to avoid false positives on resources that happen to have a `data` field (score 0.35 alone < 0.50), but permissive enough to catch well-known API patterns (known key 0.35 + any one other signal ≥ 0.50).
3. **Wrapper metadata storage**: Store `WrapperMeta` (pagination info, counts) in the collection's metadata file. This preserves context about the original API response shape.
4. **No nested wrapper detection**: The initial implementation only checks top-level keys. Nested wrappers (`{"response": {"items": [...]}}`) are out of scope — they're rare and much harder to detect without false positives.
5. **No user-extensible key lists yet**: The initial implementation uses hardcoded key lists. User-extensible lists (via config) can be added later.
6. **Reuse existing collection infrastructure**: `createCollectionEntry` and `createCollectionItems` are already factored out from the NDJSON path. The wrapper path reuses them directly.

### Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| False positives on resources with `data`/`items` fields | Threshold 0.50 requires ≥2 signals; attribute blocklist; many-scalar-fields penalty |
| Performance impact of scoring every imported JSON object | Scoring is O(n) where n = number of top-level keys (typically <20); negligible vs. CUE unification |
| Breaking existing NDJSON import path | NDJSON is detected first (line 181) before wrapper detection runs — no interference |
| Missing domain-specific wrapper keys (e.g., `users`, `pods`) | Start with high-precision known keys; can add domain keys later with lower scores |

## Future Development

### Phase 1: Analytical Layer (Next Priority)
The single most impactful work is building features that turn PUDL from "a place data goes" into "a tool that tells me things." Resource identity tracking is now in place as the prerequisite.

1. **`pudl diff`** - Compare two versions of the same resource (resource_id + version)
2. **`pudl summary`/`pudl stats`** - Aggregate views ("47 EC2 instances, 3 outliers")
3. **Basic outlier detection** - Given N instances of a schema, identify unusual field values

### Phase 2: Schema Intelligence
1. **Two-tier schema system** - Broad type recognition + policy compliance
2. **Schema drift detection** - "This resource used to validate, now it doesn't"
3. **Schema coverage reports** - "37% of data matches a specific schema, 63% is generic"

### Phase 3: Correlation & Cross-Source
1. **Cross-source correlation** - Link AWS resources to K8s resources
2. **Temporal tracking** - Same resource across multiple imports (enabled by resource_id + version)
3. ~~**Resource identity**~~ - ✅ Implemented in identity tracking

### Phase 4: Advanced Analytics
1. **DuckDB/Parquet integration** - Analytical query engine for large datasets
2. **Expert system components** - Automatic detection of common substructures
3. **Dashboard/reporting interfaces** - Visual representation of infrastructure state

## Remaining Cut Candidates

These items were identified in the project review but not yet addressed:

- `op/` + `internal/cue/processor.go` + `cmd/process.go` - CUE custom function processor (unrelated to core purpose)
- `cmd/setup.go` - Shell integration (premature convenience optimization)
- `cmd/module.go` - Thin wrapper around `cue mod` commands
- ~~`internal/streaming/` - CDC-based streaming parser~~ — Fixed and working (2026-02-09)

## Core Packages

- `internal/importer/` - Data import logic
- `internal/identity/` - Resource identity extraction and computation
- `internal/schema/` - Schema loading and management
- `internal/validator/` - CUE validation, cascade validation, validation service
- `internal/inference/` - Schema inference engine
- `internal/schemaname/` - Schema name normalization
- `internal/schemagen/` - Schema generation from data
- `internal/database/` - SQLite catalog
- `internal/config/` - Configuration loading
- `internal/init/` - Repository initialization
- `internal/git/` - Git operations for schema repo
- `internal/idgen/` - Proquint ID generation
- `internal/errors/` - Error types
- `internal/ui/` - Output formatting
- `internal/doctor/` - Health checks
- `internal/lister/` - List/query operations
- `cmd/` - CLI command definitions

---
id: pudl-yqt
title: "Built-in schema for API collection wrapper responses"
type: research
status: complete
date: 2026-02-18
tags: [schema, collection, inference, api-response, wrapper]
---

# Research: Built-in Schema for API Collection Wrapper Responses

## Problem Statement

Many APIs return collections wrapped in an object rather than as a bare array. For example:

```json
{"Items": [{"id": 1}, {"id": 2}], "Count": 2}
{"data": [{"name": "a"}], "next_token": "abc123"}
{"results": [{"key": "val"}], "total": 100, "page": 1}
```

Pudl currently only recognizes NDJSON files as collections. A JSON response like the above would be classified as a single object (matching `#Item` or a specific schema), not as a collection of items. We need a way to detect these wrapper patterns and treat the embedded array as a collection.

However, we must avoid false positives: a single resource that happens to have an array field (e.g., `{"name": "server-1", "tags": ["web", "prod"]}`) is NOT a collection wrapper.

## Current State of Collection Detection

### What Works

1. **NDJSON detection** (`internal/importer/detection.go:84-127`): Files with multiple root-level JSON objects per line are detected as NDJSON and imported as collections.

2. **Collection import flow** (`internal/importer/importer.go:599-640`): NDJSON files get a `#Collection` entry plus individual `#Item` entries for each element.

3. **Schema filtering by CollectionType** (`internal/inference/heuristics.go:71-83`): The heuristics system can filter schemas based on whether data is a collection or item, using `InferenceHints.CollectionType`.

4. **Built-in schemas** (`internal/importer/bootstrap/pudl/core/core.cue`):
   - `#Item`: Universal catchall (priority 0, accepts anything)
   - `#Collection`: Collection metadata schema (priority 75, requires `collection_id`, `item_count`, etc.)

### What's Missing

1. **No wrapper detection**: When a JSON object like `{"Items": [...], "Count": 2}` is imported, it's treated as a single object. There is no logic to recognize it as a collection wrapper.

2. **CollectionType hint not passed during inference**: At `importer.go:230-232` and `importer.go:844-846`, the `InferenceHints` struct is created without setting `CollectionType`, so the collection/item filtering in `heuristics.go:71-83` never activates.

3. **`#Collection` schema doesn't match wrapper patterns**: The existing `#Collection` schema requires pudl-internal fields (`collection_id`, `original_filename`, `format`, `item_count`) that API responses don't have. It's designed for pudl's own collection metadata, not for raw API responses.

4. **No way to unwrap and import items**: Even if we detected a wrapper, there's no pipeline to extract the array elements and import them as individual collection items (analogous to what happens for NDJSON).

## Survey of Common API Collection Wrapper Patterns

### Key Name Patterns

Based on prevalent API designs:

| Key Name | Used By | Frequency |
|----------|---------|-----------|
| `items` / `Items` | AWS DynamoDB, Google APIs, generic REST | Very common |
| `data` | Stripe, Facebook Graph, JSON:API | Very common |
| `results` | Elasticsearch, Django REST, generic search APIs | Common |
| `records` | Salesforce, Airtable | Common |
| `entries` | Contentful, generic feed APIs | Moderate |
| `objects` | S3 ListObjects, generic APIs | Moderate |
| `resources` | FHIR, generic APIs | Moderate |
| `hits` | Elasticsearch | Moderate |
| `values` | Azure, Google Sheets | Moderate |
| `list` / `elements` | Various | Less common |
| Domain-specific (`users`, `pods`, `instances`) | Most REST APIs | Very common |

### Sibling Key Patterns (Pagination / Metadata)

Collection wrappers frequently include sibling keys that signal "this is a list response":

- **Count/total**: `count`, `total`, `totalCount`, `total_count`, `totalResults`, `resultCount`
- **Pagination cursors**: `next_token`, `nextToken`, `NextToken`, `cursor`, `next_cursor`, `continuation_token`, `marker`, `NextMarker`
- **Page info**: `page`, `page_size`, `pageSize`, `per_page`, `limit`, `offset`, `has_more`, `hasMore`
- **Links**: `_links`, `links`, `next`, `previous`, `first`, `last` (HATEOAS / JSON:API)
- **Metadata**: `metadata`, `meta`, `_metadata`, `response_metadata`

### Structural Patterns

1. **Simple wrapper**: `{"items": [...]}` — single array field
2. **Wrapper + count**: `{"items": [...], "count": N}`
3. **Wrapper + pagination**: `{"items": [...], "next_token": "...", "count": N}`
4. **Envelope pattern**: `{"data": [...], "meta": {...}, "links": {...}}`
5. **Nested wrapper**: `{"response": {"items": [...], "count": N}}` — less common, harder to detect

## Heuristics for Distinguishing Wrappers from Single Resources

The core challenge: `{"tags": ["web", "prod"], "name": "server-1"}` has an array field but is NOT a collection wrapper.

### Proposed Heuristic Signals

**Strong positive signals** (high confidence the object is a collection wrapper):

1. **Dominant array field**: One field contains an array of objects, and that array contains ≥2 items with a homogeneous structure (same or similar keys). The array's element count is a significant portion of the object's "weight."

2. **Known wrapper key name**: The array field's key matches a known collection key name (`items`, `data`, `results`, `records`, `entries`, `objects`, `resources`, `hits`, `values`).

3. **Pagination siblings**: Sibling keys match known pagination patterns (`next_token`, `total`, `count`, `page`, `has_more`, `cursor`, `offset`, `limit`, `_links`).

4. **Count field matches array length**: A sibling numeric field's value equals or is close to the array's length (e.g., `"count": 5` with 5-element array).

**Weak positive signals** (contribute but not sufficient alone):

5. **Few top-level keys**: Wrapper objects typically have few keys (2-5). A response with 10+ top-level keys is more likely a single resource.

6. **Array field is the largest field by data volume**: The array constitutes the bulk of the response.

7. **Array elements are objects (not primitives)**: Arrays of strings/numbers are less likely to be collection items.

**Negative signals** (indicates NOT a wrapper):

8. **Multiple array fields of similar size**: Resources often have several array properties (tags, roles, addresses). Wrappers usually have exactly one "main" array.

9. **Many non-metadata scalar fields**: If the object has many string/number fields beyond what looks like pagination metadata, it's more likely a resource.

10. **Array field name is a known attribute name**: Names like `tags`, `labels`, `permissions`, `roles`, `addresses`, `emails` are attributes, not collection wrappers.

### Scoring Algorithm Sketch

```
score = 0.0

# Strong signals
if array_key in KNOWN_WRAPPER_KEYS:       score += 0.35
if has_pagination_siblings:                score += 0.25
if count_field_matches_array_length:       score += 0.20
if array_elements_are_homogeneous_objects: score += 0.15

# Weak signals
if top_level_key_count <= 5:               score += 0.05
if array_is_dominant_field:                score += 0.05

# Negative signals
if multiple_similar_arrays:                score -= 0.40
if array_key in KNOWN_ATTRIBUTE_KEYS:      score -= 0.30
if many_scalar_fields (>6):                score -= 0.15

# Threshold
if score >= 0.50: classify as collection wrapper
```

## Options for Implementation

### Option A: Detection + Unwrap at Import Time (Recommended)

Add a wrapper detection step in `importer.go` between data analysis and schema inference. When a wrapper is detected:

1. Extract the array of items from the wrapper
2. Import the items as a collection (reusing `createCollectionItems`)
3. Store wrapper metadata (pagination info, etc.) in the collection entry
4. Pass `CollectionType: "item"` hint when inferring item schemas

**Where to integrate**: After `analyzeDataStreaming()` returns at `importer.go:203-206`, before schema inference at `importer.go:230`. A new function like `detectAndUnwrapCollection(data) (items []interface{}, wrapperMeta map[string]interface{}, isWrapper bool)`.

**Pros**: Reuses existing collection infrastructure. Items get individual entries and schemas. Clean separation.
**Cons**: Adds complexity to the import path. Must handle edge cases carefully.

### Option B: Built-in CUE Schema for Wrapper Pattern

Define a new CUE schema `#CollectionWrapper` that structurally matches wrapper patterns using CUE constraints. This would be a fallback schema that matches objects with a single dominant array-of-objects field.

```cue
#CollectionWrapper: {
    _pudl: {
        schema_type: "collection_wrapper"
        resource_type: "generic.collection_wrapper"
        cascade_priority: 50
    }
    // Must have at least one field that is an array of objects
    // CUE can express this but it's awkward for "any key name"
    ...
}
```

**Pros**: Uses existing schema infrastructure. No special import logic.
**Cons**: CUE can't easily express "has exactly one field matching pattern X" without knowing the key name. Would need to be very permissive (high false positive risk) or require specific key names (missing custom key names).

### Option C: TypePattern-based Detection

Add a new `TypePattern` in `typepattern/` that detects collection wrappers, similar to how Kubernetes and AWS resources are detected. The pattern would use structural analysis rather than specific field names.

**Pros**: Fits the existing type detection framework. Could generate schemas automatically via `schemagen`.
**Cons**: TypePattern works on `map[string]interface{}` and checks field names/values — it's designed for known resource types, not structural patterns. Would need extension.

### Option D: Hybrid — TypePattern Detection + Import Unwrap

Combine Option A and Option C: Use a TypePattern to detect wrappers (with the scoring heuristics above), then unwrap at import time.

**Pros**: Detection is modular and testable. Unwrapping reuses collection infrastructure.
**Cons**: Most implementation effort.

## Recommendation

**Option A (Detection + Unwrap at Import Time)** is the best approach because:

1. **It solves the actual problem**: Users importing API responses will get proper collection entries with individual items, enabling per-item schema inference, search, and tracking.

2. **It reuses existing infrastructure**: The NDJSON collection import path already handles creating collection entries + items. We can factor that out and reuse it.

3. **It's incremental**: Can start with well-known wrapper key names + pagination signal detection (high precision), then expand to structural analysis later.

4. **The CollectionType hint gap gets fixed naturally**: The unwrap step would set `CollectionType: "item"` when inferring schemas for extracted items.

### Suggested Implementation Plan

1. **Create `internal/importer/wrapper.go`** — wrapper detection logic:
   - `DetectCollectionWrapper(data map[string]interface{}) *WrapperDetection`
   - Contains the scoring heuristics, known key lists, pagination detection
   - Returns the array field key, extracted items, wrapper metadata, confidence score

2. **Integrate into import path** in `importer.go`:
   - After `analyzeDataStreaming()`, if data is `map[string]interface{}`, call `DetectCollectionWrapper`
   - If detected with sufficient confidence, route to collection import path
   - Pass wrapper metadata (pagination info) as collection metadata

3. **Fix CollectionType hint propagation**:
   - In `assignItemSchema()`, pass `CollectionType: "item"` in hints
   - In collection entry creation, pass `CollectionType: "collection"` in hints

4. **Add known key name and attribute name lists** — well-curated lists based on the survey above, with ability for users to extend via configuration later.

5. **Tests**: Unit tests for wrapper detection with positive cases (various API patterns) and negative cases (resources with array attributes).

### Known Wrapper Key Names (Initial List)

```go
var knownWrapperKeys = map[string]bool{
    "items": true, "Items": true,
    "data": true, "Data": true,
    "results": true, "Results": true,
    "records": true, "Records": true,
    "entries": true, "Entries": true,
    "objects": true, "Objects": true,
    "resources": true, "Resources": true,
    "hits": true, "Hits": true,
    "values": true, "Values": true,
    "elements": true, "Elements": true,
    "list": true, "List": true,
    "rows": true, "Rows": true,
    "nodes": true, "Nodes": true,
    "edges": true, "Edges": true,
}
```

### Known Attribute Key Names (Negative Signal)

```go
var knownAttributeKeys = map[string]bool{
    "tags": true, "labels": true,
    "permissions": true, "roles": true,
    "addresses": true, "emails": true,
    "groups": true, "scopes": true,
    "features": true, "capabilities": true,
    "attachments": true, "dependencies": true,
    "headers": true, "cookies": true,
    "args": true, "arguments": true,
    "env": true, "environment": true,
    "ports": true, "volumes": true,
    "rules": true, "conditions": true,
}
```

### Known Pagination Key Names

```go
var paginationKeys = map[string]bool{
    "next_token": true, "nextToken": true, "NextToken": true,
    "cursor": true, "next_cursor": true, "nextCursor": true,
    "continuation_token": true, "ContinuationToken": true,
    "marker": true, "NextMarker": true, "next_marker": true,
    "page": true, "page_size": true, "pageSize": true,
    "per_page": true, "perPage": true,
    "offset": true, "limit": true,
    "total": true, "totalCount": true, "total_count": true,
    "count": true, "Count": true,
    "has_more": true, "hasMore": true, "HasMore": true,
    "_links": true, "links": true,
    "next": true, "previous": true,
    "meta": true, "metadata": true, "_metadata": true,
}
```

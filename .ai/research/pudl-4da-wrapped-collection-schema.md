---
id: pudl-4da
title: Wrapped Collection Fallback Schema
status: complete
created: 2026-02-18
author: Spark
labels: [research, schema, collection]
---

# Wrapped Collection Fallback/Catchall Schema

## Summary

This research investigates how to add a "wrapped collection" fallback/catchall schema to pudl that matches JSON objects with array fields like `Items` or `results`. The schema should be an open struct accepting additional fields.

## Problem Statement

Many APIs return collections wrapped in an object rather than as bare arrays. Currently, pudl handles:
- **NDJSON collections**: Newline-delimited JSON files with one object per line
- **JSON arrays**: Top-level arrays of items (`[{...}, {...}]`)
- **#Collection schema**: Internal representation for imported collections

However, pudl does not have native handling for **wrapped collections** - JSON objects that contain an array field with the actual data plus optional metadata fields.

## Common Wrapped Collection Patterns

### AWS/Cloud Provider Patterns
| Field Name | Example Source | Description |
|------------|---------------|-------------|
| `Items` | DynamoDB Scan/Query | Array of database items |
| `Contents` | S3 ListObjects | Array of S3 objects |
| `Reservations` | EC2 DescribeInstances | Array of reservation groups |
| `Resources` | Various AWS services | Generic resource array |
| `Tags` | Most AWS resources | Array of tag objects |

### REST API Standard Patterns
| Field Name | Example Source | Description |
|------------|---------------|-------------|
| `results` | Django REST Framework | Primary results array |
| `data` | JSON:API, many APIs | Primary data array |
| `records` | Airtable, various | Database records |
| `items` | Generic pagination | Collection items (lowercase) |
| `entries` | Contentful CDA | Content entries |
| `values` | Google APIs | Array of values |

### GraphQL Patterns
| Field Name | Example Source | Description |
|------------|---------------|-------------|
| `edges` | Relay-style pagination | Array of edge objects |
| `nodes` | Direct node access | Array of node objects |

### Common Metadata Fields
| Field Name | Purpose |
|------------|---------|
| `NextToken`, `nextToken` | AWS pagination cursor |
| `count`, `total`, `totalCount` | Total item count |
| `pageInfo`, `pagination` | Pagination metadata object |
| `_links` | HAL/HATEOAS links |
| `meta`, `metadata` | General metadata |
| `errors` | Error information |

## Proposed Schema Design

### CUE Schema: `#WrappedCollection`

```cue
package core

// WrappedCollection matches JSON objects that wrap an array in a known field.
// This is a catchall for API responses like AWS CLI output, REST API responses,
// and GraphQL connection results.
#WrappedCollection: {
    // PUDL metadata for cascading validation
    _pudl: {
        schema_type:      "wrapped_collection"
        resource_type:    "generic.wrapped_collection"
        cascade_priority: 50  // Higher than Item (0), lower than Collection (75)
        cascade_fallback: ["pudl.schemas/pudl/core:#Item"]
        identity_fields:  []
        tracked_fields:   []
        compliance_level: "permissive"
    }
    
    // The schema validates by structural detection:
    // - Must have at least one top-level field that is an array
    // - Array field name should match common collection patterns
    // 
    // Accept any structure (open schema)
    ...
}
```

### Type Detection Pattern

A new type pattern should be added to detect wrapped collections:

```go
// wrappedcollection.go - new file in internal/typepattern

// Common collection field names (case-insensitive matching recommended)
var wrappedCollectionFields = []string{
    // AWS patterns
    "Items", "Contents", "Reservations", "Resources", "Objects",
    "Buckets", "Functions", "Instances", "Volumes", "Images",
    // REST patterns  
    "results", "data", "records", "items", "entries", "values",
    "list", "rows", "documents", "elements",
    // GraphQL patterns
    "edges", "nodes",
}

// Metadata field names that boost confidence
var wrappedCollectionMetadataFields = []string{
    "NextToken", "nextToken", "next_token",
    "count", "total", "totalCount", "total_count",
    "pageInfo", "page_info", "pagination",
    "meta", "metadata", "_meta",
    "_links", "links",
}
```

## Detection Algorithm

```go
func detectWrappedCollection(data map[string]interface{}) string {
    // 1. Find array fields
    var arrayFields []string
    for key, value := range data {
        if _, isArray := value.([]interface{}); isArray {
            arrayFields = append(arrayFields, key)
        }
    }
    
    // 2. No arrays = not a wrapped collection
    if len(arrayFields) == 0 {
        return ""
    }
    
    // 3. Check if any array field matches known patterns
    for _, field := range arrayFields {
        if isKnownCollectionField(field) {
            return "wrapped-collection:" + field
        }
    }
    
    // 4. Fallback: if there's exactly one array field and metadata fields exist,
    //    treat it as a wrapped collection
    if len(arrayFields) == 1 && hasMetadataFields(data) {
        return "wrapped-collection:" + arrayFields[0]
    }

    return ""
}
```

## Integration Points

### 1. Schema Addition (`internal/importer/bootstrap/pudl/core/core.cue`)

Add `#WrappedCollection` as a new fallback schema in the core module, between `#Item` (priority 0) and `#Collection` (priority 75). Suggested priority: **50**.

### 2. Type Pattern Registration (`internal/typepattern/wrappedcollection.go`)

Create a new pattern file following the pattern established by `gitlab.go`:
- Use empty `RequiredFields` since detection is structural
- Use `TypeExtractor` for custom detection logic
- Set `Priority: 40` (lower than kubernetes:100, aws-cloudformation:80, gitlab-ci:70)

### 3. Importer Enhancement (`internal/importer/importer.go`)

When a wrapped collection is detected:
1. Extract the array from the wrapper field
2. Import each item as an individual catalog entry (like NDJSON handling)
3. Store the wrapper metadata separately or in collection metadata

### 4. Schema Name Fallback (`internal/schemaname/schemaname.go`)

Update `IsFallbackSchema` to recognize `pudl/core.#WrappedCollection`:

```go
func IsFallbackSchema(name string) bool {
    normalized := Normalize(name)
    return normalized == "pudl/core.#Item" ||
        normalized == "pudl/core.#Collection" ||
        normalized == "pudl/core.#WrappedCollection" ||  // Add this
        normalized == "pudl/core.#CatchAll"
}
```

## Cascade Hierarchy

After implementation, the fallback hierarchy would be:

```
Most Specific (highest priority)
    ↓
kubernetes (100) - k8s resources
    ↓
aws-cloudformation (80) - CloudFormation resources
    ↓
gitlab-ci (70) - CI/CD pipelines
    ↓
#Collection (75) - NDJSON/multi-record files
    ↓
#WrappedCollection (50) - API responses with array fields [NEW]
    ↓
#Item (0) - Universal fallback
    ↓
Most Generic (lowest priority)
```

## Example Data Matching

### Would Match `#WrappedCollection`

```json
{
  "Items": [
    {"id": "1", "name": "Item 1"},
    {"id": "2", "name": "Item 2"}
  ],
  "Count": 2,
  "ScannedCount": 2
}
```

```json
{
  "results": [
    {"title": "Post 1"},
    {"title": "Post 2"}
  ],
  "count": 2,
  "next": "https://api.example.com/posts?page=2"
}
```

### Would NOT Match (falls to #Item)

```json
{
  "id": "123",
  "name": "Single Resource",
  "tags": ["a", "b"]
}
```
Note: Has an array field (`tags`), but it's not a known collection field name and there's no pagination metadata.

## Implementation Considerations

### Open Schema Requirement

The schema must accept additional fields (`...` in CUE) to handle the diverse metadata patterns across different APIs. This is consistent with `#Item` and `#Collection`.

### Array Field Extraction

When importing, pudl should:
1. Identify the primary collection field
2. Import each array element as a separate catalog entry
3. Preserve wrapper metadata in collection-level metadata

### Priority Tuning

The priority should be:
- **Higher than #Item** (0) - wrapped collections are more specific
- **Lower than #Collection** (75) - #Collection is for pudl's internal format
- **Lower than ecosystem-specific patterns** - AWS, k8s, etc. should match first

Suggested value: **50**

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| False positives on objects with incidental array fields | Require known field names OR pagination metadata |
| Performance impact of checking all patterns | Wrapped collection check should be O(n) on field count |
| Breaking existing imports | New schema has lower priority than existing patterns |

## Recommendation

Implement `#WrappedCollection` as described, with:

1. **Priority 50** for cascade ordering
2. **Open struct** (`...`) to accept any fields
3. **TypeExtractor-based detection** (like GitLab CI)
4. **Array field naming heuristics** for classification

This provides a useful intermediate fallback between the universal `#Item` catchall and ecosystem-specific schemas, enabling better handling of common API response formats.

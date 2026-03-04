# Collections

PUDL handles multi-record data through its collection system. When you import a file containing multiple items, PUDL creates a parent **collection entry** linked to individual **item entries**.

## How Collections Are Created

Collections are created automatically during import in two cases:

### 1. NDJSON Files

Files with newline-delimited JSON objects are detected by format:

```json
{"id": "item1", "type": "resource", "data": "..."}
{"id": "item2", "type": "resource", "data": "..."}
{"id": "item3", "type": "resource", "data": "..."}
```

```bash
pudl import --path cloud-inventory.json
# Detected format: ndjson
# Created collection with 832 items
```

### 2. JSON API Wrapper Responses

JSON objects that look like API response envelopes are automatically detected and unwrapped:

```json
{
  "items": [
    {"id": "i-abc", "type": "ec2"},
    {"id": "i-def", "type": "ec2"}
  ],
  "count": 2,
  "next_token": "eyJ..."
}
```

```bash
pudl import --path api-response.json
# Detected collection wrapper (score: 0.75)
# Created collection with 2 items
```

## Wrapper Detection

The wrapper detection algorithm scores JSON objects for signals that they're collection envelopes rather than actual resource data:

| Signal | Score | Condition |
|--------|-------|-----------|
| Known wrapper key | +0.35 | Array field named `items`, `data`, `results`, `records`, `entries`, `resources`, `hits`, `values`, etc. |
| Pagination siblings | +0.25 | Sibling fields like `next_token`, `cursor`, `total_count`, `has_more`, `page`, `limit`, `_links` |
| Count matches length | +0.20 | A numeric sibling field equals the array length |
| Homogeneous elements | +0.15 | ≥80% of array elements share the same top-level keys |
| Few top-level keys | +0.05 | Total keys ≤ 5 |
| Dominant array | +0.05 | The array is the largest field by estimated size |

Negative signals prevent false positives:

| Signal | Score | Condition |
|--------|-------|-----------|
| Known attribute key | -0.30 | Array field named `tags`, `labels`, `permissions`, `roles`, `env`, `ports`, etc. |
| Multiple similar arrays | -0.40 | ≥2 array fields of similar size |
| Many scalar fields | -0.15 | >6 non-pagination scalar fields |

**Threshold**: Score ≥ 0.50 triggers unwrapping. This requires at least two positive signals, preventing false positives on resources that happen to have a `data` or `items` field.

## Collection Structure

```
Collection entry (📦) — schema: pudl/core.#Collection
    ├── Item 0 (📄) — individually schema-inferred
    ├── Item 1 (📄) — individually schema-inferred
    └── Item 2 (📄) — individually schema-inferred
```

Each item gets:
- Its own catalog entry with a unique content hash and proquint ID
- Its own schema assignment (inferred independently)
- Its own resource identity (`resource_id` and `version`)
- A reference back to the parent collection via `collection_id` and `item_index`

The collection entry stores:
- The original file (for provenance)
- Item count and schema distribution
- Source metadata (file size, import timestamp, origin)
- Wrapper metadata (pagination info, counts) if detected as a wrapper

## Querying Collections

### List collections

```bash
# All collections
pudl list --collections-only

# Collections by format
pudl list --collections-only --format ndjson
```

### List items

```bash
# All items (excluding collection entries)
pudl list --items-only

# Items from a specific collection
pudl list --collection-id cloud-inventory

# Items from a collection filtered by schema
pudl list --collection-id cloud-inventory --schema aws/ec2.#Instance
```

### Cross-collection queries

```bash
# Find all instances of a schema across all collections
pudl list --schema aws/ec2.#Instance

# All items of a schema type
pudl list --items-only --schema aws/ec2.#Instance
```

### Inspect individual items

```bash
# Show an item's details
pudl show fusaf-gofag

# Show raw data of an item
pudl show fusaf-gofag --raw
```

## Deleting Collections

Collections have special deletion semantics to prevent accidental data loss:

```bash
# Attempting to delete a collection with items fails by default
pudl delete govim-nupab
# Error: Collection 'govim-nupab' has 5 items
# Use --cascade to delete the collection and all its items

# Delete collection AND all its items
pudl delete govim-nupab --cascade --force

# Delete a single item from a collection (always works)
pudl delete fusaf-gofag --force
```

## Collection Display

Collections and items are visually distinguished in `pudl list` output:

```
1. cloud-inventory [pudl/core.#Collection] (2025-10-06) 📦
   Origin: cloud-inventory | Format: ndjson | Records: 832 | Size: 890.9 KB

2. cloud-inventory_item_0 [aws/ec2.#Instance] (2025-10-06) 📄
   Origin: cloud-inventory_item_0 | Format: json | Records: 1 | Size: 1.2 KB
   Collection: cloud-inventory [#0]
```

## Generating Schemas from Collections

When you want to create a schema for items within a collection:

```bash
# Generate a schema for the item type (not the collection wrapper)
pudl schema new --from govim-nupab --collection --path mypackage/#MyItem
```

This analyzes the items in the collection and generates a CUE schema matching their common structure.

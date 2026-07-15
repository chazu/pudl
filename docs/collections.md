# Collections

PUDL handles multi-record data through its collection system. When you import a file
containing multiple items, PUDL creates a **collection entry** linked to individual
**item entries**. Membership is normalized separately from the catalog item, so the
same content-addressed item can be included in multiple collections.

## How Collections Are Created

Collections are created automatically during import for newline-delimited JSON:

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

## Typed envelopes

Typed JSON emitted by mu or another producer can carry schema metadata around a
payload:

```json
{
  "schema": {"module": "mu/aws", "version": "v1", "definition": "#Instance"},
  "definitions": [{"path": "aws.cue", "content": "..."}],
  "data": {"id": "i-abc", "type": "ec2"}
}
```

PUDL recognizes an envelope only when `schema.module`, `schema.version`, and `data`
are present. It records the canonical schema reference in `item_schemas`, caches
inline definitions, and imports the inner `data` value. An envelope is metadata
around a payload; it is not inferred to be a collection. Use NDJSON when each line
should become a collection item.

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
- A membership row in `collection_memberships(collection_id, item_id, item_index)`

The collection entry stores:
- The original file (for provenance)
- Item count and schema distribution
- Source metadata (file size, import timestamp, origin)

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

Collections have special deletion semantics to prevent accidental data loss. Cascade
deletion removes the collection's membership rows. An item is deleted only when no
other collection still references it:

```bash
# Attempting to delete a collection with items fails by default
pudl delete govim-nupab
# Error: Collection 'govim-nupab' has 5 items
# Use --cascade to delete the collection and its memberships

# Delete collection and memberships; orphaned items are removed
pudl delete govim-nupab --cascade --force

# Delete a single item entry (memberships are cleaned up)
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
# Generate a schema for the item type (not the collection entry)
pudl schema new --from govim-nupab --collection --path mypackage/#MyItem
```

This analyzes the items in the collection and generates a CUE schema matching their common structure.

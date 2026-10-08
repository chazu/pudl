`--schema` with a `#` matches whole definition names: `pudl/gcp.#Route`,
`gcp.#Route` and `#Route` all find routes but not `#Router`. A value without
`#` (`gcp`, `Route`) is a substring match. `--origin` and `--format` are
substring matches; `%` and `_` match literally.

`--json` prints one object, not an array, and it is **paginated** (`--per-page`
defaults to 20):

```json
{
  "entries": [{"id": "…", "proquint": "…", "schema": "…", "origin": "…",
               "format": "…", "size_bytes": 0, "record_count": 0,
               "import_timestamp": "…", "stored_path": "…", "metadata_path": "…",
               "confidence": 1, "collection_type": "item", "collection_id": "…",
               "item_id": "…", "item_index": 0}],
  "total_entries": 0, "total_matched": 0, "total_pages": 0, "current_page": 1,
  "summary": {"total_size_bytes": 0, "total_records": 0, "unique_schemas": 0,
              "unique_origins": 0, "unique_formats": 0}
}
```

Count matches with `total_matched` (not `entries | length`). `summary` sizes
and record totals cover the current page; its `unique_*` counts cover the whole
catalog; it is omitted when nothing matches.

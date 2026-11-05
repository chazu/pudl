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

`--all` returns every match in one result and still respects `--limit`. It cannot
be combined with `--page`. `--where path=value` filters decoded payloads using
typed equality; wildcards match any element, and repeated conditions are ANDed.
`--select path` includes chosen fields (repeatable). These operations inspect
individual records, skipping collection containers. Filtering precedes pagination.
Unreadable payloads fail the command. Inspection has a 64 MiB per-record and
256 MiB total scan budget; narrow schema or collection scope for larger data.
`--fancy` remains available and can display the filtered record set.

`--count-by schema` (or `origin`, `format`) prints how many matching entries
share each value instead of listing them, most common first. Every filter
(`--schema`, `--origin`, `--format`, `--where`, `--artifacts`, collection
filters, `--global`) and `--limit` applies; it cannot be combined with
`--page`, `--per-page`, `--select` or `--fancy`. pudl's own record types
(`pudl/core.#ObserveSnapshot`, `pudl/systemmodel.#SystemModel`, legacy mu
artifacts) are listed after the resource types under `pudl bookkeeping:`.
With `--json`:

```json
{
  "count_by": "schema",
  "groups": [{"schema": "pudl/gcp.#Firewall", "count": 12},
             {"schema": "pudl/core.#ObserveSnapshot", "count": 3, "bookkeeping": true}],
  "total_entries": 15, "total_matched": 15
}
```

Each group is keyed by the counted field (`schema`, `origin` or `format`);
`bookkeeping` appears only when true. `total_entries` is the number counted
(`total_matched` capped by `--limit`); `groups` is `[]` when nothing matches.

# `pudl list --count-by` — pudl-kdl

**Date**: 2026-11-04
**Type**: Feature
**Status**: Complete

## Summary

`pudl list` can now count catalog entries per schema (or origin, or format)
instead of listing them, so "how many of each type are in the catalog?" no
longer needs `pudl list --all --json | jq ...`. Counting reuses the list query,
so every list filter applies. pudl's own record types are separated from
resource types so snapshots and system models do not dominate the output.

## Public CLI surface

- `pudl list --count-by FIELD` with FIELD `schema`, `origin` or `format`
  (shell completion offers these). Any other value is an input error.
- Filters that apply: `--schema`, `--origin`, `--format`, `--where`,
  `--artifacts`, `--collection-id`, `--item-id`, `--collections-only`,
  `--items-only`, `--global`; `--limit` caps the entries counted (in
  `--sort-by` order). `--all` is accepted and has no further effect.
- Mutually exclusive with `--page`, `--per-page`, `--select`, `--fancy`.
- Origins are grouped as list displays them: collection items count under
  their collection's proquint, not one group per item.

Text output (most common first, ties by value; bookkeeping types last):

```
6 entries in 3 schemas

COUNT  SCHEMA
    3  pudl/aws.#Bucket
    1  pudl/git.#GitRepository

pudl bookkeeping:
    2  pudl/core.#ObserveSnapshot
```

`--json`:

```json
{"count_by": "schema",
 "groups": [{"schema": "pudl/aws.#Bucket", "count": 3},
            {"schema": "pudl/core.#ObserveSnapshot", "count": 2, "bookkeeping": true}],
 "total_entries": 5, "total_matched": 5}
```

Each group is keyed by the counted field. `bookkeeping` is present only when
true (schema counts only). `total_entries` is the number counted
(`total_matched` capped by `--limit`). No matches gives `"groups": []`.

Bookkeeping types: `pudl/core.#ObserveSnapshot`, `pudl/systemmodel.#SystemModel`
and the legacy mu artifacts (`pudl/mu.#Manifest`, `#ManifestAction`,
`#PlanOutput`, `#DriftObservation`), compared after `schemaname.Normalize`.

## Files

- `cmd/list_count.go` (new): count keys, bookkeeping classification,
  aggregation (`countListEntries`), text and JSON rendering (`runListCount`).
- `cmd/list.go`: `--count-by` flag, help text, exclusivity, completion, and the
  dispatch after filters are built. Catalog opening is unchanged.
- `cmd/list_count_test.go` (new): text table, JSON groups, `--where`,
  `--schema`, `--origin`, `--limit`, empty result, invalid field, `--page`
  conflict.
- `cmd/cli_json_contract_test.go`: `list --count-by schema` (with and without
  matches) in the JSON contract run.
- `docs/cli/notes/list.md`, `docs/cli-reference.md` (regenerated),
  `cmd/guide/query.txt` (COUNT section), `cmd/guide/agents.txt`,
  `skills/pudl-core/SKILL.md` and its embedded copy, `cmd/testdata/help.golden.json`.

## Notes

- Counting loads every matching catalog row (no payloads unless `--where` is
  given, which keeps its 64 MiB / 256 MiB inspection budget).

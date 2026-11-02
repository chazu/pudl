# Check report: populate counts observed and new records — pudl-xcc

A check report's `populate.records` counted only the records that became new
catalog entries. Records are content-addressed, so a record re-observed
unchanged is linked into the new snapshot and not stored again. A collector
re-run over unchanged resources therefore reported `records: 0` while
`pudl snapshot show` reported the full count, and operators read the 0 as
"collected nothing".

## Change

- `observe.ObserveIngestResult` now has two counts:
  - `Records`: the distinct records linked into the snapshot, whether stored
    new or re-observed unchanged. It equals the membership count
    `pudl snapshot show` reports (byte-identical records in one observation
    share an entry and are counted once).
  - `NewRecords`: the records stored as new catalog entries (what `Records`
    meant before).
  `PreparedObservation.Commit` (`internal/observe/prepare.go`) counts both in
  the same pass. Nothing is re-queried.
- `checkreport.Populate` gains `NewRecords` (`new_records`), set by
  `operation.(*modelRun).ingest` from the ingest result. `Records` keeps its
  JSON name and now means records observed.
- The text report's populate line reads `- records: 55 (0 new)`.

## JSON contract

`members[].report.populate`:

```json
{
  "target": "//models/my-model:populate",
  "records": 2,
  "new_records": 0,
  "snapshot_id": "snap_..."
}
```

`records` changed meaning, from new entries to records observed in the
snapshot. `new_records` is new and always present (`0` is meaningful).

## Callers

The only non-test caller of `IngestObserve` that reads the count is
`internal/operation/populate.go`. Tests that asserted the old dedup semantics
(`internal/observe/ingest_test.go`, `shared_record_test.go`,
`internal/operation/failed_inputs_test.go`) now assert both counts.

## Tests

- `cmd/gcp_pack_live_test.go`: a second `pudl check gcp-firewalls` over the
  unchanged fake gcloud data reports `records: 2`, `new_records: 0`, and
  `records` equals `pudl snapshot show <id> --json`'s `records`.
- `internal/checkreport/report_test.go`: the markdown shows both counts.

## Docs

`docs/observation-scope.md` and the `pudl-core` skill describe both fields.

# Observed entries get the next version of their resource (pudl-42w)

**Date:** 2026-11-04

## Root cause

`observe.PreparedObservation.Commit` (`internal/observe/prepare.go`) never set
`CatalogEntry.Version` on the new entries it stored, so `AddEntry` defaulted
every observed entry to version 1. The import paths (`importer/document.go`,
`importer/collection_stream.go`) already read `GetLatestVersion` inside the
transaction; the observe path did not.

The model instance written by `recordModelInstance` goes through this ingest,
so when a model's definition changed (the issue's repro adds
`_gcpAssetScope: "off"`), the new instance kept the same resource_id
(identity = name) and got a second version 1. The bug was not specific to
model instances: any observed resource whose content changed between
observations was recorded as another version 1.

## Work done

- `Commit` now reads the resource's latest version inside the commit
  transaction and stores a new entry as `latest + 1`. Two states of one
  resource in a single observation get consecutive versions. Deduplicated
  (unchanged) records add no version.
- New opt-in repair `versions` (`internal/repair/versions.go`): for each
  resource with two entries holding the same version, it renumbers that
  resource's entries 1..N in import order (tie-break created_at, then id), using
  the existing `assignVersions`. Resources whose versions are already unique
  are not touched. It is opt-in (`pudl doctor --fix --only versions`), because
  doctor's policy is that historical versions are not renumbered
  automatically. A repair is warranted because the bug left duplicates in
  every catalog where an observed resource changed, and re-observing (doctor's
  old advice) does not remove them.
- The doctor "Duplicate historical resource versions" warning now names that
  repair in its fix hint.
- `pudl doctor --help`, `docs/cli/notes/doctor.md`, the generated
  `docs/cli-reference.md` and `cmd/testdata/help.golden.json` list the repair.

Verified end to end with the issue's repro: the changed model instance is now
version 2 and doctor reports unique versions. A catalog made by the old
binary shows the warning, `--fix` skips the opt-in repair, and
`--fix --only versions` renumbers the second instance to v2.

Not changed: when a re-observed unchanged entry gains an identity
(`UpdateEntryIdentity` in `Commit`), it moves to a new resource_id without
being re-versioned. That path is separate from this bug, and `ReassignEntry`
shows how it could join the chain at its end.

## Public API

- CLI: `pudl doctor --fix --only versions` (new repair name `versions`, opt-in).
- No Go API changes outside `internal/`.

## Tests

- `internal/observe/version_test.go`:
  `TestIngestObserve_ChangedRecordIsNextVersion` (fails before the fix with
  versions `[1 1 1 1]`).
- `internal/repair/versions_test.go`: `TestVersionsRepair` (dry run writes
  nothing, renumbers in import order, leaves unique histories alone,
  idempotent, opt-in).

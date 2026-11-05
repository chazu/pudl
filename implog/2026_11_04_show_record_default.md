# `pudl show` prints the record by default — pudl-kii

**Date**: 2026-11-04
**Type**: Feature
**Status**: Complete

## Summary

`pudl show ID` used to print only catalog details and compliance; the stored
record needed `--raw`, `--field` or `--json --raw`. It now prints the record by
default, after the details and compliance section. `--meta-only` restores the
previous output. `--raw` is still accepted (it is now the default), and
`--field`, `--history` and `--metadata` behave as before. Records are printed as
stored: sensitive fields were redacted at import, and show does not change that.

## Public CLI surface

- `pudl show ID`: details, compliance, then a `RAW DATA` section with the
  stored record. Stored JSON is indented with `json.Indent`, so values (large
  integers included) and key order are unchanged. Earlier `--raw` output
  decoded into Go values first, which sorted keys and rounded large numbers.
  YAML is re-rendered as before; other formats are printed verbatim.
- `--meta-only` (new): details and compliance only. It can be combined with
  `--metadata`. It is mutually exclusive with `--raw`, `--field` and
  `--history`.
- `--raw`: still accepted, now the default. Help text: "Show the stored
  record (the default; kept for compatibility)".
- `pudl show ID --json`: the same object as before (`entry`, `compliance`,
  `metadata` with `--metadata`). It now includes `raw` (or `raw_text` for
  formats other than JSON) by default, where before it needed `--raw`. Nothing
  was renamed or removed. `--meta-only --json` gives the old default shape,
  with no `raw` or `raw_text`.

## Files

- `cmd/show.go`: `--meta-only` flag and its exclusions, the new default,
  updated help text. The metadata section uses the shared section header.
- `cmd/show_data.go` (new): `printShowSection`, `printEntryData` and
  `formatStoredData` (record rendering, moved out of `displayDetailedEntry`).
- `cmd/show_json.go`: doc comment.
- `cmd/show_data_test.go` (new): the record is shown by default with exact
  numbers and key order; `--raw` gives the same output; `--meta-only` with and
  without `--metadata`; flag conflicts; JSON `raw` by default and absent with
  `--meta-only`; a sensitive field stays redacted in text and JSON.
- `cmd/cli_json_contract_test.go`: `show --meta-only` added to the contract run.
- Docs: new `docs/cli/notes/show.md`, `docs/cli-reference.md` (regenerated),
  `README.md`, `docs/getting-started.md` (the walkthrough step is now plain
  `pudl show RECORD_ID`), `docs/gcp-network-hygiene.md`.
- Guide and hints: `cmd/guide/observe.txt`, `cmd/guide/policies.txt`, and
  `cmd/prime.go` (the loop step now says show prints the record).
- `skills/pudl-core/SKILL.md` and its embedded copy;
  `cmd/testdata/help.golden.json`.

## Notes

- Because `--json` now reads the record by default, an entry whose stored file
  is missing makes `show --json` fail (as `--json --raw` did before). Use
  `--meta-only --json` to read the catalog details of such an entry.
- `docs/gcp-cataloging-workflow-findings.md` mentions `--raw` and its
  `RAW DATA` header. It is a findings record, so it was left unchanged; the
  header is still `RAW DATA`.

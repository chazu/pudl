# Structured, fail-closed inventory drift

**Date:** 2026-10-05

**Issue:** advances pudl-qrl (explain drift findings with consistent human and
JSON reports).

## Problem

The inventory set-diff lived in `cmd/run_inventory.go` and had three defects:

- A desired record with no resolvable identity was skipped (`continue`). A model
  whose records all lacked identity reported **clean, verified** having compared
  nothing.
- Observed records sharing an identity silently overwrote each other, so the
  comparison used whichever was read last.
- `fieldsDiffer` reported only the first mismatching field, chosen by Go map
  iteration order, and compared with `fmt.Sprint`, so `1` and `"1"` were equal.

Findings were a single `Diff` string, and the human report rendered drift with
its own code, so it could not show what the JSON lacked (and vice versa).

## Change

The comparison moved to `internal/acute`:

- `inventory_diff.go` — `RecordIdentity`, `InventorySetDiff` over
  `[]ObservedRecord`. Fails closed: an identity-less desired record is
  `unidentifiable`; several *differing* observed records with one identity are
  `ambiguous` (identical duplicates count once). Both are drift.
- `drift_compare.go` — `CompareDesired` reports every unsatisfied desired field
  in sorted path order. Top level is ensure-present (extra observed fields are
  fine); nested values compare exactly, reporting extra nested observed keys as
  `unexpected` — the same strictness the whole-value `fmt.Sprint` comparison had.
  `ValuesEqual` is typed: numbers compare by exact value across Go types
  (`int64`, `float64`, `json.Number`, big values), and never equal strings.
- `drift_render.go` — `ModelDriftResult.WriteMarkdown`, the single human renderer
  for the drift section, reading the same fields JSON serializes.

`cmd/run_inventory.go` keeps `identityResolver`/`recordIdentity` as thin aliases
(used by `model_derive.go`), decodes observed records with `UseNumber`, and
carries each record's import time and the scope's snapshot (ID and creation
time) into the result. The differential (k8s) path keeps its classification and
now stamps the live observation time; stored drift observations include
structured `fields` when present. The human run report also shows the populate
snapshot ID, `verified`, `observation_id`, `needs_verification`, and mutation
receipts, which were JSON-only.

## Public API (internal/acute)

```go
const DriftMissing, DriftChanged, DriftUnidentifiable, DriftAmbiguous = "missing", "changed", "unidentifiable", "ambiguous"

type FieldDiff struct {
	Path       string `json:"path"`
	Expected   any    `json:"expected,omitempty"`
	Observed   any    `json:"observed,omitempty"`
	Missing    bool   `json:"missing,omitempty"`
	Unexpected bool   `json:"unexpected,omitempty"`
}
func (FieldDiff) String() string // compact summary, used for Diff
func (FieldDiff) Detail() string // "path: expected X, observed Y" (JSON values)

type ResourceDrift struct { Resource, Reason string; Fields []FieldDiff; ObservedAt *time.Time; Diff string }
type ModelDriftResult struct { Clean bool; Drifted []ResourceDrift; Verified bool; ObservationID, SnapshotID string; ObservedAt *time.Time }
func (*ModelDriftResult) StampObservedAt(time.Time)
func (ModelDriftResult) WriteMarkdown(io.Writer)

type IdentityResolver func(schema string) []string
type ObservedRecord struct { Data map[string]any; ObservedAt *time.Time }
func RecordIdentity(rec map[string]any, identity IdentityResolver) (key, label string, ok bool)
func InventorySetDiff(desired []map[string]any, observed []ObservedRecord, identity IdentityResolver) []ResourceDrift
func CompareDesired(desired, observed map[string]any) []FieldDiff
func ValuesEqual(a, b any) bool
func FormatValue(v any) string
```

## Compatibility

`Diff` keeps its format (`default_branch: release → want main`); several fields
join with `; `, and values print as JSON only when their plain forms would read
as equal (`port: "1" → want 1`). Models whose desired records lack identity, or
whose snapshots hold conflicting records for one identity, now report drift
instead of clean.

## pudl-qrl status

Met: human and JSON expose the same findings; expected and observed values per
field; observation time and snapshot/observation references; missing identity
represented explicitly. Remaining: previous-observation values (needs a prior
snapshot lookup), failed-check detail in the same finding view, and `--json`
purity for every walkthrough command.

## Tests

`internal/acute`: unidentifiable desired set is not clean; differing duplicates
are ambiguous, identical ones are not; every field mismatch reported in
deterministic path order; `1` vs `"1"` changed; nested and list paths; exact
large-integer comparison; human rendering states every JSON finding.
`cmd`: real-catalog structured fields and timestamps, unidentifiable end to end,
run report human/JSON parity. `make test-git-walkthrough` passes unchanged.

# cass-memory substrate — Curator (`pudl facts curate`)

Date: 2026-06-14

## Summary

The deterministic Curator stage of the ACE loop (plan §5): advance observation
maturity from accumulated feedback, no LLM, same rules every run. This is the last
pure-pudl piece before the mu-side orchestration.

cass's Curator also does dedup and conflict detection; in pudl those are already
handled or deferred — exact dedup is intrinsic (content-addressed fact IDs), and
conflict-graph resolution is the deferred dlktk path. So v1 Curator = the
feedback-driven maturity transitions.

## Behavior

Per non-terminal observation, tallying helpful/harmful feedback across its version
lineage:

```
harmful >= --reject-harmful (default 2)                          -> rejected
status raw,      helpful >= --promote-helpful (default 3), harmful 0 -> reviewed
status reviewed, helpful >= --promote-helpful,             harmful 0 -> promoted
```

Maturity advances one step per run, so a well-supported observation reaches
`promoted` over successive runs (raw → reviewed → promoted). `--dry-run` previews.

### Lineage (the correctness subtlety)

A maturity transition mints a new content-addressed fact ID, which would orphan
feedback that targeted the prior version. Fixed by recording a `prevVersion`
pointer on every transition (`promoteFact`) and walking that chain when tallying
(`observationLineage`). So 3 helpful given on a `raw` observation still count after
it transitions to `reviewed` and drive the `reviewed -> promoted` step. Verified by
CLI e2e and the schema now carries `prevVersion?: string`.

## Changes

### `internal/importer/bootstrap/pudl/nous/nous.cue`
- `#Observation` gains `prevVersion?: string` (lineage pointer).

### `cmd/facts_write.go`
- Extracted the promote transaction into a reusable `promoteFact(db, idArg, to,
  rule) (oldID, newID, error)`; `facts promote` now calls it. `promoteFact` records
  `prevVersion = <prior fact ID>` on each transition.

### `cmd/facts_curate.go` (new)
- `pudl facts curate [--promote-helpful N] [--reject-harmful N] [--dry-run]`:
  tallies current feedback by target, walks each observation's `observationLineage`,
  applies the rules above via `promoteFact`.

### `cmd/prime.go`, `cmd/guide.go`
- Documented `pudl facts curate`.

## Verification

- `cmd/facts_curate_test.go` (new): `promoteFact` records `prevVersion` and the new
  status; `observationLineage` returns [new, original]; illegal `raw → promoted`
  rejected.
- CLI e2e: 3 helpful on a raw obs → run1 raw→reviewed, run2 reviewed→promoted
  (lineage carried the raw-id feedback), run3 no-op; 2 harmful → rejected;
  `--dry-run` previews without writing.
- `CGO_ENABLED=0 go test ./...` full suite green.

## Notes / deferred

- Weighting: cass applies a 4× harmful multiplier. v1 uses plain counts with
  separate promote/reject thresholds (reject threshold is lower, approximating the
  asymmetry). A weighted score can replace the counts later.
- Conflict detection between contradictory promoted rules is deferred to the dlktk
  path (plan §14).

Remaining: the pith-target orchestration (Generator `pudl memory context`,
`pudl memory cycle`, `pudl hooks`, and the mu `-C/--root` flag + `~/.pudl/mu.cue`).

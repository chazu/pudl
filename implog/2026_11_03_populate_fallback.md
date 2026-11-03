# Populate fallback: a second source for the same resources (pudl-q4v.3)

**Date:** 2026-11-03

## Work done

A `#CommandObserve` populate can declare `fallback: {when, observe}`: another
way to observe the same resources, used when its own runs cannot answer. The
GCP Cloud Asset Inventory collectors will search CAI first and fall back to
the gcloud list commands when the asset API is unavailable.

The fallback is used instead of the populate's runs when:

- a run exits non-zero with stderr matching a `when` pattern (checked before
  `empty_when`; timeouts and undecodable output never match);
- the `each` command fails that way; or
- `each` lists no inputs (credentials that see no organization).

The first trigger stops the primary: later runs are skipped, records already
read are discarded and their spool and staging budget released. The two
sources are never mixed. This replaces the design's first idea of falling
back per input: primary and fallback usually fan out over different things
(organizations versus projects), so inputs do not correspond.

The fallback has its own runs (with transforms), each, timeout, fail_fast and
empty_when; its records are routed by the populate's schema; its failures are
failed inputs. It cannot set a schema or have a fallback of its own.

## Public API

**CUE** (`internal/systemmodel/schema.cue`):

- `#CommandSource`: `runs`, `timeout?`, `each?`, `fail_fast?`, `empty_when?`
  (now shared by a populate and its fallback)
- `#CommandObserve: {#CommandSource, schema?, allow_schema_fallback?,
  fallback?: {when: [string, ...string], observe: #CommandSource}}`

**Go:**

- `systemmodel.Fallback{When []string; Observe Populate}`,
  `(*Fallback).Patterns()`, `Populate.Fallback *Fallback`; the loader
  validates patterns, the fallback's placeholders and empty_when, attaches
  its runs' transforms, and exempts them from concreteness
- `populate.FallbackUse{Reason, Pattern, StderrLine, Trigger *FailedInput}`,
  `Summary()`, reasons `FallbackFailedInput` (`failed_input`),
  `FallbackEachFailed` (`each_failed`), `FallbackNoInputs` (`no_inputs`)
- `populate.Result.Fallback`, `checkreport.Populate.Fallback`
  (JSON `populate.fallback`); the markdown report prints
  `- source: fallback (...)`, and stderr gets
  `pudl: using the populate fallback: ...`

## Moved to pudl-q4v.4

The GCP pack settings planned for this ticket (`_gcpAssetScope`,
`_gcpBillingProject`, organization discovery) land with the first Cloud
Asset collector in `pudl-q4v.4`. Until then nothing would run or test them.

## Tests

- `internal/populate/fallback_test.go`: a matching failure replaces the
  populate (later runs never start), fallback before empty_when, unmatched
  failures and timeouts do not trigger, each failure and no inputs trigger
  (the fallback fans out with its own each), unmatched each failures still
  fail, the fallback's failures and empty inputs, a fresh staging budget
  (fails without the reset).
- `internal/systemmodel/fallback_test.go`: loading a fallback with
  transforms; eight invalid fallbacks rejected.
- `cmd/command_fallback_e2e_test.go`: `pudl model validate` and
  `pudl check` with a disabled-API search falling back to per-project
  listings; JSON and markdown reports name the fallback.

## Files

- `internal/systemmodel/fallback.go` (new), `systemmodel.go`,
  `transform.go` (transforms and concreteness for fallback runs),
  `schema.cue`
- `internal/populate/fallback.go` (new), `command.go`, `empty.go`,
  `enumerate.go`
- `internal/checkreport/model.go`, `populate.go`; `internal/operation/populate.go`
- Docs: `docs/observation-scope.md`, `skills/pudl-core/SKILL.md` and its
  embedded copy, design doc §1
- The repository workspace's built-in `systemmodel.cue` refreshed

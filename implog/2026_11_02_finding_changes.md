# Finding changes over time (pudl-xf5.5)

Each check now records its failing matches as findings with stable keys and
classifies them against the model's comparable earlier reports: `new`,
`persisting`, `resolved`, or `unknown` when this evaluation cannot establish
their absence, each with a first-seen time. `pudl report ID --since ID2`
compares two named reports, and `pudl check --fail-on new` fails only on new
failures.

## Design

**Baseline: stored reports, not snapshots evaluated again.** The previous
observation is the model's earlier stored reports (`run_reports`, by model,
newest first, up to 50). They are the findings pudl recorded and showed, under
the rules in force then; reading them is cheap; and they survive snapshot
retention, whereas re-evaluating an earlier snapshot needs it retained, rebuilds
projections, and would rewrite history whenever a rule changed. The cost: a
rule change shows as resolved plus new findings.

**Identity.** `FindingKey` is a SHA-256 digest (24 hex) of the check head's
arguments without evidence identifiers (`entry_id`, `evidence_id`,
`observation_id`, `snapshot_id`, `run_id`), so row order and re-observation do
not change it. A head that projects only evidence identifiers is identified by
them. `policy_noncompliant(resource, policy, severity)` findings are therefore
one per resource and policy (verified in a test). A failing `expect: nonempty`
check is one finding with no arguments. Witnesses carry their `key`.

**Recording every match.** Witnesses stay bounded at 20, but the comparison
needs every failing match, so a check records `findings` for all of them up to
`FindingLimit` (1000), sorted by key, with arguments where the match is also a
witness. More matches than that mark `findings_truncated`.

**Comparable.** Same model, same mode (an inventory replay — drift without a
populate — is its own mode), same check name, query, expectation and check
scope, and the same observation scopes when both name evidence. Earlier
evaluations without a verdict are skipped.

**Complete.** An evaluation can resolve only if it reached a verdict, recorded
every match (not truncated), and its scope was completely observed: every
selected snapshot complete (`snapshots`), its own populate snapshot complete
(`run`), or always for `latest-known` (a partial observation keeps earlier facts
known, so a match disappears only when facts change). Failed fan-out inputs
(`pudl-xf5.3`) make run and latest-known checks unknown and snapshots
incomplete before classification, so they leave findings unknown.

**Classification.** History is the comparable evaluations with a verdict,
newest first, back to and including the newest complete one. A current match
any of them recorded is `persisting` (first seen: the earliest record),
otherwise `new` (first seen: now). A recorded match not seen now is `resolved`
when the current evaluation is complete, else `unknown`. Incomplete reports in
between count as having seen what they recorded but never as having established
absence. With no comparable report every failing match is new.

**Waivers (`pudl-xf5.4`).** A waived match keeps its key and state and is
marked `waived`; `changes.waived` counts them. Waiving never resolves a
finding. An expired waiver excuses nothing, so its finding counts again.
`SetFindings` records findings while it matches waivers (the matcher counts
matches, so it is not called twice).

**When.** `operation` classifies in `finalize`, after checks, failed-input
marking and waivers, before the report is summarized and saved. A failure to
read history is a warning; the report still records every finding.

## CLI surface

- `pudl check ... --fail-on any|new` (default `any`). With `new`, a check
  whose only failures are fail-severity checks without a new unwaived finding
  succeeds (and says so on stderr); every model must have run. Composes with
  `--detailed-exitcode`: 0 instead of 2; drift still 2; errors, checks without a
  verdict, and blocked or cancelled models unchanged.
- `pudl report [ID] --since ID2 [--json]`: classifies the report's (or the
  newest's) checks against report ID2, models paired by name; a check is not
  compared when ID2 lacks it, reached no verdict, or evaluated it differently.
  One flag on the existing command was the leanest surface.
- Human report: each classified check prints `since <run> (<time>): N new, N
  persisting, N resolved, N unknown`, then each finding with arguments and
  first-seen time (and its waiver). Summaries add `findings: N new, N
  resolved`.

## Public API

`internal/checkreport`:
- `Finding{Key, State, FirstSeen, Waived, Arguments}`; states `FindingNew`,
  `FindingPersisting`, `FindingResolved`, `FindingUnknown`; `FindingLimit`.
- `FindingKey(args)`, `MatchFindings(tuples, expect, outcome, witnesses,
  waivedKeys)`.
- `CheckResult.Findings`, `FindingsTruncated`, `Changes`; `Witness.Key`.
- `Changes{Since, SinceAt, New, Persisting, Resolved, Unknown, Waived, Note}`.
- `Observation{RunID, Mode, At, Check, Complete}`, `NewObservation(runID,
  mode, at, check, populateComplete)`, `Comparable(current, earlier)`,
  `Classify(current, history) ([]Finding, *Changes)`.
- `RecordChanges(db, report, at)`.
- `Compare(db, current, since *Stored) (*Comparison, error)`; `Comparison`,
  `ModelChanges`, `CheckChanges`, `(*Comparison).WriteText(w)`;
  `(*Stored).ID()`.
- `WriteChanges(w, check)`.

`internal/acute`: `VerdictSummary.NewFindings`, `ResolvedFindings`.

`internal/database`: `(*CatalogDB).ModelRunReports(model, limit)`.

`internal/operation`: `Result.Drifted`, `Result.NewFailures`,
`(*Result).OnlyKnownCheckFailures()`.

`cmd`: `--fail-on` on `check` (`failOnAny`, `failOnNew`, `failOnExit`,
`detailedCheckExit(result, err, onlyNew)`); `--since` on `report`.

## Tests

- `internal/checkreport/changes_test.go`: baseline, regression, fix sequence
  (new, persisting, resolved sets and first-seen); a partial observation and a
  check without a verdict leave findings unknown, and a later complete one
  resolves; truncated findings cannot resolve; completeness by scope;
  comparability (query, observation scope, mode, replay); policy violations by
  resource and policy; older reports compared by witnesses; key ignores
  evidence IDs; bounds and nonempty findings; waived and expired-waiver
  findings.
- `internal/operation/changes_test.go`: only new unwaived fail-severity
  findings are new failures; failed inputs leave findings unknown.
- `cmd/check_changes_test.go`: CLI journey — baseline, regression, unchanged,
  partial observation, fix — with `--fail-on new` (with and without
  `--detailed-exitcode`), human report and `report --since` (JSON and not
  comparable).
- `cmd/run_exit_test.go`: the `--fail-on new` exit mapping.

## Line deltas against `sre-focus`

Go files, `git diff --numstat sre-focus` after the final merge:

| | Added | Removed | Net |
| --- | --- | --- | --- |
| Production | +925 | −33 | +892 |
| Tests | +500 | −7 | +493 |

## Verification

`CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 mise exec -- make lint` (0
issues) and `mise exec -- make check-docs check-skills` pass.

## Follow-ups

- Drift findings are not classified over time; `--fail-on new` leaves drift
  counting as before.
- `run_reports` has no index on `model`; the history read scans by
  `created_at`. Add one if catalogs hold many reports.

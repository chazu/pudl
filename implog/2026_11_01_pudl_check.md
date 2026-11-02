# `pudl check MODEL...`: one command, one report (pudl-4fx.1)

`pudl check MODEL...` replaces `pudl run` and `pudl run set`. It observes and
evaluates exactly the named models and records one report whether it names one
model or several. `pudl report [ID]` (top level) replaces `pudl run report`.
Value bindings were removed rather than carried over.

## Command surface

| Before | After |
| --- | --- |
| `pudl run <model>` | `pudl check <model>` |
| `pudl run set <a> <b>...` | `pudl check <a> <b>...` |
| `pudl run report [ID]` | `pudl report [ID]` |
| `pudl run --populate 'command:…'` | `pudl check --populate 'command:…'` |
| `--from-catalog --catalog-scope S` | unchanged on `check`; names exactly one model |
| `--detailed-exitcode`, `--check-upstream`, `--progress-json` | unchanged on `check` |
| `--max-observation-age` (`run`, `run set`) | removed with value bindings |
| `pudl status` | unchanged |

`pudl run …` now fails with `unknown command "run"` and cobra suggests `check`
(`SuggestFor`). The mapping is in `docs/retired-commands.md#run-and-run-set`.

## Behavior

- **Exactly the named models, producers first.** `acute.NewRunSetPlan` takes
  `RunSetModel{Name, DependsOn, Aliases}` and adds an edge only for a
  `depends_on` that names another model in the set. Dependencies outside the
  set add no edge and never run. Cycles, duplicates and self-dependencies fail
  before anything runs. A model whose named producer did not succeed is
  recorded `blocked`; an interrupt records the rest `cancelled`. These are the
  former `run set` semantics, now also used for one model.
- **One report.** Every check, of one model or many, writes one aggregate
  report to `run_set_reports` with `report_version: 3` and a `check_…` ID. Each
  member carries its result, run ID, summary and, new in version 3, its full
  report as `members[].report`. The member reports are also stored as before in
  `run_reports`, so `pudl report <run_id>`, model status, evidence pins and
  `pudl model list` work unchanged. The JSON field names of the version-2 set
  report (`run_set_id`, `members`, `ordered_members`, `edges`, `plan_digest`,
  `summary`) are kept.
- **Output.** `--json` writes only the aggregate report. Human output prints
  each model's plan and report as it concludes (`# model: <name>`), then the
  summary (`check <id>: <status>` and one line per model). The populate target
  (`//models/<name>:populate`) is no longer printed in human output; it stays in
  JSON under `members[].report.populate.target`.
- **Errors and exit codes.** A check fails when any model did not succeed:
  `check <id> failed: <model> <result>: <error>; …`. The error wraps each
  member's error, so `errors.Is(err, errFailSeverityChecks)` still holds.
  `--detailed-exitcode`: 0 clean, 2 drift or failing fail-severity checks only,
  1 anything else (`detailedCheckExit`, replacing `detailedRunExit` and
  `detailedRunSetExit`).
- **Reading reports.** `pudl report` reads check reports (each member's
  markdown, then the summary), member reports by run ID, and stored `run` and
  `run set` reports (versions 1 and 2), with the legacy-field rendering kept.
- **Progress.** `--progress-json` also emits `check` started/completed/failed
  events, so a check that fails while resolving models still reports progress.

Hooks merged from `sre-focus` survive: `checkModelSchemas` (pudl-o5i) runs from
`resolveRunModel` for every named and ad-hoc member before any command runs,
and from `pudl model validate`; `drift.identity_fallbacks` and its notes
(pudl-167) appear in each member report; populate secret warnings
(pudl-xf5.8.3) stay on the member report.

## Binding decision: removed

Evidence that nothing relied on value bindings:

- **Examples.** Neither `examples/git-inventory` nor
  `examples/gcp-network-hygiene` declares `inputs` or `bindings`.
- **Smoke and walkthrough.** No smoke test or getting-started step used
  `run set` or bindings.
- **Skills and docs.** The `pudl-core` skill, `concepts.md` and
  `schema-authoring.md` described bindings but no workflow used them.
- **Only users.** Their own tests (`internal/wiring`, the run-set integration
  test, the template tests) and the repository's
  `.pudl/schema/models/kick_tires.cue` fixtures. Those fixtures also used
  `plugins:` and had stopped loading when mu plugins were retired.

Removed: `internal/wiring`; template `inputs`/`bindings` decoding,
`@pudl(binding=...)` classification (`ClassifyBinding`, `BindingClass`,
`ValueBinding`) and `Elaborate`/`ElaborateJSON`; `CatalogDB.LatestSuccessfulObserveSnapshot`
and `SuccessfulObserveSnapshotForRun`; binding dependency facts reconciliation;
binding evidence and unresolved-binding fields in the run report;
`--max-observation-age`; the `@pudl(binding=plain)` attributes in the built-in
schemas; and the kick-tires fixtures. `ModelTemplate.Model()` decodes a template
concretely. `inputs` and `bindings` join the retired model fields, so a model
that declares them fails to load with an error naming the field. The schema
keeps their shape only so those files compile.

Argument count never changes binding policy, because no binding policy remains.
Ordering comes only from `depends_on` within the named set, whether one model
is named or many.

Kept: the `snapshot_reuse_blocks` table and migration. Retention still uses it,
so restored observations do not count as a model's latest live observation
until it is observed again. Existing `model_depends_on` facts with source
`binding:<model>` stay as history; `pudl model deps` labels them `binding`.

## `pudl status` kept

`pudl report` reads one operation's recorded findings. `pudl status` shows the
current verdict recorded on each target across all models, which no report
answers. Its overlap is with the status column of `pudl model list`, not with
`report`, so it is out of scope here.

## Public API

- `cmd`: `checkCmd` (`check <model>...`), `reportCmd` (`report [ID]`),
  `executeCheck(ctx, args, checkOptions) (*checkResult, error)`,
  `detailedCheckExit`, `printCheckReport`, `checkReportVersion = 3`.
- `internal/acute`: `RunSetModel{Name, DependsOn, Aliases}`,
  `RunSetPlan.Digest(mode)`, `RunSetMemberReport.Report json.RawMessage`,
  `NewCheckID()` (replaces `NewRunSetID`).
- `internal/systemmodel`: `ModelTemplate{Name, Origin, DependsOn}` and
  `ModelTemplate.Model()`. Retired fields gain `inputs` and `bindings`.

## Line deltas against `sre-focus`

Go files, `git diff --numstat sre-focus HEAD` after the final merge of
`sre-focus`:

| | Added | Removed | Net |
| --- | --- | --- | --- |
| Production | +629 | −1611 | −982 |
| Tests | +273 | −785 | −512 |

Per commit: bindings removal prod net −950, tests net −600; `pudl check`
prod net −91, tests net +29; docs and smoke tests net +32 (tests).

## Verification

`CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 mise exec -- make lint` (0
issues), `mise exec -- make check-docs check-skills test-git-walkthrough` and
`go test -tags=smoke ./test/smoke/` pass.

## Follow-ups

- An opt-in strict identity mode for `pudl check`, suggested by the pudl-167
  log, was not added.
- `pudl status` and the status column of `pudl model list` overlap.
- `pudl-4fx.4`: move check orchestration out of `cmd/`.

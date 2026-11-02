# Check and report orchestration moved out of `cmd/` (pudl-4fx.4)

`cmd/` now parses flags, builds the workspace and output the internal packages
take, renders, and maps exit codes. Check execution, report assembly and
reading, inventory drift, snapshot comparison, command populate, the model
dependency graph, and doctor's catalog checks and repair pass live in internal
packages with their own tests. No behavior changed: help text, `--json`
output, `testdata/help.golden.json` and `docs/cli-reference.md` are unchanged.

## Inventory of what moved

| Former `cmd/` file | Now |
| --- | --- |
| `check_execute.go` (plan, members, synthetic members, report save, status) | `internal/operation/check.go` |
| `run_execute.go` (model resolution, run context, phases) | `internal/operation/model_run.go` |
| `run_conclude.go`, `run_report.go` (verdict, status write, report persist) | `internal/operation/conclude.go` |
| `run_record.go`, `run_catalog.go` | `internal/operation/record.go`, `catalog.go` |
| `run_checks.go` (checks adapter) | `internal/operation/checks.go` |
| `run_populate.go`, `run_resolve.go` (ingest, model instance) | `internal/operation/populate.go` |
| `run_depends.go` (graph) | `internal/modeldeps`; wrappers in `internal/operation/dependencies.go` |
| `run_options.go`, `run_adhoc.go` | `operation.Options`/`ValidateOptions`; `systemmodel.AdHocModel` |
| `run_command_populate.go` | `internal/populate/command.go` |
| `run_inventory.go`, `inventory_identity.go`, `snapshot_compare.go` (logic) | `internal/inventory` |
| `run_report.go` (types, markdown), `run_summary.go`, `run_check_witnesses.go`, `run_plan_render.go`, `run_report_legacy.go`, `report.go` (reading) | `internal/checkreport` |
| `run_resolve.go` (template resolution), `command_line.go` | `internal/systemmodel/resolve.go`, `command_spec.go` |
| `doctor_catalog.go`, `doctor_repairs.go` (repair loop) | `internal/doctor/catalog_check.go`, `internal/repair/pass.go` |

What stays in `cmd/`: flags and `Args` validation (`check.go`, `doctor*.go`),
`invocationWorkspace()` (globals to `operation.Workspace`), the JSON-or-text
choice (`printCheckReport`, `readOperationReport`, `compareSnapshots`), progress
encoding (`run_progress.go`), exit codes (`run_exit.go`), interrupt handling
and the temp-dir registry (`run_workspace_cleanup.go`), repair preview/apply
argv and doctor rendering, and `resolveModel`/`listModels` for `pudl model`.

Output streams, progress and rendering are injected: `operation.Output` carries
`Out`, `Err`, `Live`, `Progress` and a `Concluded` hook that `cmd` uses to
print the check report at the moment it always printed (before the final
progress event). Configuration loads lazily through `Workspace.LoadConfig`, so
a check that never needs schemas never loads them, as before. No internal
package imports `cmd`.

## Public API

`internal/operation` — `pudl check` execution.
- `Check(ctx, Workspace, Output, Options) (*Result, error)`; `Result{Report
  *acute.RunSetReport, Findings bool}`, `(*Result).OnlyCheckFailures()`.
- `Options{Models, PopulateSpec, FromCatalog, CatalogScope, CheckUpstream}`,
  `ValidateOptions(Options) error`.
- `Workspace{PudlDir, Name, ModelDirs, RulePaths, ModelRulePaths, LoadConfig,
  SchemaPaths, IngestLimits, TrackTempDir}` with `Projections()`,
  `CheckModelSchemas(m)`, `InventoryPolicy()`.
- `Output{Out, Err, Live, Progress, Concluded}`.
- `ReportVersion = 3`, `ErrFailSeverityChecks`.

`internal/checkreport` — the recorded result of a check.
- `ModelReport` (was `cmd.RunReport`), `Populate`, `CheckResult`
  (`Uncertain()`), `Witness`, `ModelReportVersion = 2`.
- `(*ModelReport).Render(asJSON)`, `Markdown()`; `Summarize(r, db)`,
  `SummarizeCheck(db, check)`, `DecodeModelReport(payload)`.
- `Witnesses(tuples, expect)`, `WitnessLimit`, `WriteFindings(w, result)`.
- `WriteCheck(w, check, members)`, `RenderPlan(model)`.
- `Find(db, id)`, `Latest(db)` → `*Stored{Payload, Check, RunID, Evidence}`
  with `JSON()` and `WriteText(w)`; `Evidence`.

`internal/inventory` — catalog observations compared.
- `Drift(ctx, db, scope, desired, Policy)`, `LoadObserved(ctx, db, scope)`,
  `Observed`, `ScopeFilter(scope, lookupErr)`.
- `Policy{Identity, Namespace}`, `LoadPolicy(schemaPaths)`, `Namespace`,
  `NormalizeRecord(record, namespace)`.
- `CompareSnapshots(ctx, db, before, after, loadPolicy) (*Comparison, error)`,
  `Comparison`, `Change`.

`internal/populate` — `#CommandObserve` execution.
- `Run(ctx, systemmodel.Populate, Request) ([]any, error)`;
  `Request{ModelDir, Limits, Stderr, TrackTempDir}`; `DefaultTimeout`.

`internal/modeldeps` — the `model_depends_on` graph.
- `Relation`, `DeclaredSource(model)`, `Resolver`, `RegisteredIn(dirs)`,
  `Declared(model, deps, resolve)`, `Reconcile(db, from, source, wanted)`,
  `Diff(declared, current)`, `StaleUpstreams(db, rules, model)`,
  `EdgeArgs(args)`.

Additions to existing packages:
- `systemmodel`: `ResolveTemplate(dirs, name)`, `ResolveTemplateIn(dir, name)`,
  `ShortDefName`, `InstanceDefinition`, `InstanceTarget`, `SplitCommandLine`,
  `ParsePopulateSpec`, `AdHocModel`.
- `doctor`: `CheckCatalogEntries(ctx, CatalogScan)`, `CatalogScan`,
  `CatalogDiagnostic`, `DriftEntryIDs`.
- `repair`: `RunPass(ctx, Pass) []Result`, `Pass{Selected, PudlDir, Fix,
  DryRun, Only, HealthOK, Env, AfterChange}`. It lives in `repair`, not
  `doctor`, because `internal/scrub`'s tests import `doctor` and `repair`
  imports `scrub`.
- `checks`: `ScopeRun`, `ScopeLatestKnown`, `ScopeSnapshots`.
- `importer`: `DecodeEntry(entry)`. `secretscan`: `Notes(warnings, total)`.

## Duplication deleted

- `printChecks` (unused in production; the report markdown renders checks)
  and its test.
- Three copies of the exact-number model-report decode → `DecodeModelReport`.
- The "unknown or error" outcome test → `CheckResult.Uncertain`.
- `cmd`'s `checkScopeRun`/`checkScopeGlobal` and the scope string literals in
  `checks.Evaluate` → `checks.Scope*`.
- `cmd.loadEntryPayload` and `repair.LoadPayload` → `importer.DecodeEntry`.
- `checkPasses`/`headExposesRunID` wrappers (tests only); the tests now call
  `checks.Passes`/`HeadExposesRunID`.
- `cmd`'s projection registry, model schema-contract check and inventory
  policy now delegate to `operation.Workspace`; `pudl check` and `pudl model
  deps` share `modeldeps.RegisteredIn`.

## Tests

Moved with the code and runnable without the CLI: run catalog, run record and
verdict, checks scoping, check integration (producers first, blocked
consumer, one-model report), report summaries and witnesses, inventory drift
and snapshot scoping, dependency diff, model template resolution, command-line
parsing, inference-check selection. New package tests: live output and
`Concluded`, a check with no report and its progress events, `Findings` and
`OnlyCheckFailures`, `CompareSnapshots`, `populate.Run` (decode, `set`,
failures and stderr tail, byte limit, cleanup, relative programs, timeout),
`modeldeps.Declared` and an idempotent `Reconcile`, `checkreport.Find`/`Latest`
and stored-report rendering, the repair pass (no catalog, unhealthy `--fix`,
environment failure, opt-in skip) and a missing catalog scan.

Kept in `cmd`: the JSON contract and help golden tests, CLI journeys
(`check` through `runCLI`), the report command's evidence-availability and
legacy-field tests, the exit-code mapping (now against `operation.Result`),
`--progress-json` separation, doctor CLI tests and `model validate`.

## Line counts

`cmd/` production Go: 13,026 lines in 88 files at the merge base (`d4e0408`),
9,732 in 67 before merging `sre-focus` again. After the final merges (which
brought `policy list`/`show`, `show` compliance, the scrub and the GCP pack),
`sre-focus` has 13,399 in 91 and this branch 10,106 in 70.

`git diff --numstat sre-focus HEAD`, Go files, rename-aware:

| | Added | Removed | Net |
| --- | --- | --- | --- |
| Production | +2,384 | −2,075 | +309 |
| Tests | +959 | −445 | +514 |

The production growth is the injection seam and package boundaries:
`operation/env.go` (`Options`, `Workspace`, `Output`, 145 lines with docs),
`checkreport.Stored` with `Find`/`Latest`, `populate.Request`,
`inventory.Comparison`/`Policy`, `doctor.CatalogScan`, `repair.Pass`, and a
package doc comment per new package. The test growth is the new package
tests listed above.

## Verification

After the final merge of `sre-focus`: `CGO_ENABLED=0 go test ./...`,
`CGO_ENABLED=0 mise exec -- make lint` (0 issues; the `runCatalog.Close`
errcheck exclusion now names `internal/operation`), `mise exec -- make
check-docs check-skills test-git-walkthrough`, `go test -tags=smoke
./test/smoke/` and `mise exec -- make test-race` pass.

## Follow-ups

- `cmd/run_resolve.go`'s `resolveModel` (for `pudl model show`/`validate`)
  repeats the search loop of `systemmodel.ResolveTemplate` over
  `listModelsIn`; folding it in would change which invalid models `model
  show` reports, so it was left.
- The remaining `cmd/run_*` file names (`run_exit.go`, `run_progress.go`,
  `run_resolve.go`, `run_workspace_cleanup.go`) predate `pudl check`.

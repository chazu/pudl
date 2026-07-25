# P1 defect fixes: run record, masked receipt, unpersisted observations, dry-run purity

Cleared the four P1s from the defect register. Three are fully fixed; the fourth
is fixed for every catalog mutation but leaves one scratch-file issue open, which
is recorded rather than papered over.

## Defect 5 / `pudl-r0f` — lost apply receipt masked on three of four exit routes

`manifestFailure` was only converted to `needs-verification` when the loop
happened to end clean, so the cap-exhausted, apply-error and observe-error routes
dropped it. The cap route then mapped to status `failed` — the one status D2
rejects for a receipt loss, because it invites re-applying work that already
succeeded.

- `ConvergeResult.NeedsVerification` is now **orthogonal** to `Outcome`: a lost
  receipt can accompany any terminal outcome, so it is no longer encoded as one.
- Every exit is a `break` rather than a `return`, so the needs-verification
  verdict is evaluated on all four routes.
- `runVerdict` checks it **before** the outcome, so it dominates.
- Applying and then failing to re-observe is the same operational state — the
  system changed and PUDL cannot prove the result — via a new
  `OutcomeObserveError`. An observe failure *before* any apply changed nothing and
  stays an ordinary failure.

This also fixed the in-process half of Defect 1: the observe-after-apply route no
longer returns an empty verdict.

## Defect 1 / `pudl-v5z` — interrupted run leaves a stale `clean`

`UpsertEntry` omits `status` from its UPDATE set, so an interrupted run left the
model row holding the *previous* run's status. A crashed converge could leave a
`clean` that had already been invalidated by an apply.

New `runs` table (`internal/database/catalog_runs.go`, idempotent migration):

- `StartRun` before any phase, so a run that never finishes is discoverable.
- `FinishRun` from a `defer`, so an early `return err` is still a **recorded**
  termination. A row left unfinished therefore means the process died without a
  word — the two cases are now distinguishable.
- `UnfinishedRuns` surfaces those rows, and `pudl run` warns about earlier
  unfinished runs of the same model, noting that its recorded status predates
  them.
- The run row carries `needs_verification` and a note, which is what lets an
  `unknown` caused by a lost receipt be told apart from the `unknown` of a
  resource nobody has ever observed. That was D2's outstanding dependency.

A converge run also marks its model `unknown` up front: its previous verdict
stops being trustworthy the moment it may mutate. Observe-only runs change
nothing, so they keep their last real verdict. `persistRunStatus` now reports
failures instead of discarding both the open error and the update result.

## Defect 4 / `pudl-asy` — observations that decide `clean` were never persisted

`observeDrift` ran `mu observe`, parsed a verdict, and returned — no catalog
write. The verdict drove promotion to `clean`, so the claim was unfalsifiable
afterwards.

`mubridge.RecordDriftObservation` now stores each differential observation: the
verdict, the drifted resources, and mu's raw output so the verdict can be
re-derived. `ModelDriftResult.ObservationID` and `acute.Observation.ObservationID`
carry the entry ID.

The entry type is `drift-observation`, deliberately **not** `observe`. Inventory
drift loads observed records as `entry_type='observe' AND collection_type='item'`,
so filing a drift verdict under `observe` would have let it satisfy a desired
resource — trading one false clean for another. There is a regression test for
exactly that.

Covers both the converge loop and the observe-only differential path; the latter
matters more, since its verdict drives `promoteConvergingResources` in the default
mode. A dry run stores nothing.

## Defect 6 / `pudl-j4t` — dry-run purity (partial)

`recordModelInstance` and `reconcileModelDependencies` ran unconditionally, before
the dry-run branch was reached, so `--dry-run` created snapshot entries, item
entries, collection memberships, raw files and `model_depends_on` facts. Both are
now skipped, along with the run record and drift observations. Four of the five
nouns in invariant 4 are covered.

**Left open, deliberately.** `setupReconcileWorkspace` writes `mu.cue` and the
desired manifests into a temp subdir under the mu project root, and a dry run
genuinely needs them there: `mu build --plan` reads its config from disk, and the
project-embedded design is a deliberate architecture decision. Cleanup is
deferred, so a *killed* run leaves `pudl_run_*` behind — but that is equally true
of a normal run and is not specific to `--dry-run`. Calling this defect fully
fixed would be overclaiming; what remains is a scratch-file lifecycle issue, not
a catalog mutation.

## Public API

- `pudl run`: no new flags. Behaviour changes — a converge run marks its model
  `unknown` while running; a dry run writes nothing to the catalog; unfinished
  earlier runs are warned about.
- `acute.ConvergeResult.NeedsVerification bool` — orthogonal to `Outcome`.
- `acute.OutcomeObserveError` — new outcome value. The *status* vocabulary is
  unchanged at five values.
- `acute.Observation.ObservationID string`.
- `acute.ModelDriftResult.ObservationID string`.
- `ConvergeReport.NeedsVerification bool` (`needs_verification` in `--json`).
- `database.RunRecord`, `CatalogDB.StartRun/FinishRun/GetRun/UnfinishedRuns`.
- `mubridge.DriftObservation`, `mubridge.RecordDriftObservation`,
  `mubridge.DriftObservationEntryType`, `mubridge.DriftObservationSchema`.
- `setupReconcileWorkspace` and `runDrift` take a run ID; the former also takes a
  dry-run flag. Both are package-private.
- `persistRunStatus` takes a `live bool` so warnings respect `--json`.

## Tests

- `internal/acute/coordinator_test.go` — lost receipt surviving cap exhaustion and
  a later apply failure; needs-verification when re-observe fails after an apply;
  an observe failure before any apply is *not* needs-verification. `fakeExecutor`
  gained `applyErrOn` to script a failure on a chosen iteration.
- `cmd/run_test.go` — `runVerdict` precedence: needs-verification outranks
  cap-exhausted, execute-error and observe-error; observe-error before any apply
  still maps to `failed`.
- `internal/database/catalog_runs_test.go` — start/finish lifecycle, unfinished-run
  discovery per model and globally, empty verdict still terminal, idempotent
  `StartRun` and migration, errors for unknown run and missing row.
- `internal/mubridge/drift_observation_test.go` — evidence is stored and the
  verdict re-derivable; a drift observation does **not** appear among observed
  records; identical content dedupes; empty target rejected.

`CGO_ENABLED=0 go test ./...`, `go vet ./...`, and `go test -race` on
`internal/acute` + `internal/database` all pass.

## Not fixed here

Dry-run purity is not covered by a test: asserting it end-to-end needs the
fake-mu harness from Recommendation 1D, which does not exist yet. The guard is a
plain flag check, but that is an argument, not a test, and it is worth saying so.

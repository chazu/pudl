# Convergence and mu integration removed (pudl-z87)

PUDL now discovers, catalogs and governs infrastructure; it never changes it.
The convergence workflow and every mu integration are gone. Observation runs
plain commands that print JSON (`#CommandObserve`); governance is checks,
desired-state comparison against the catalog, and reports. Design:
`docs/design/2026-11-01-remove-convergence-and-mu.md`.

## Line deltas (Go, this epic's own commits)

| Step | Production | Tests |
|---|---|---|
| `pudl-z87.1` convergence, approvals, sealed values | +375 −5045 (net −4670) | +240 −3899 (net −3659) |
| `pudl-z87.2` mu-backed observers | +130 −1367 (net −1237) | +68 −392 (net −324) |
| `pudl-z87.3` mu bridge, `internal/observe` | +103 −2202 (net −2099) | +80 −1301 (net −1221) |
| `pudl-z87.4` legacy evidence | +177 −37 (net +140) | +186 −8 (net +178) |
| `pudl-z87.5` examples, docs, tooling | +83 −182 (net −99) | +5 −1051 (net −1046) |
| **Total** | **+868 −8833 (net −7965)** | **+579 −6651 (net −6072)** |

Measured with `git diff --numstat` per commit (non-`_test.go` vs `_test.go`
Go files), excluding the merge of `sre-focus`.

## Step 1 — convergence (`pudl-z87.1`)

Removed `run --converge` and `--only`, `--dry-run`, `--max-iters`,
`--max-applies`, `--require-approval`; the converge loop
(`internal/acute/coordinator.go`, `plan.go`, `cmd/run_converge.go`); apply
budgets; `run resume`/`reject`; mutating run sets (`run_set_mutation.go`,
`run_set_mu_plan.go`, `acute/runset_mutation_plan.go`); write-ahead mutation
intents and receipts (`PrepareRunMutation`, `RecordApply`,
`Begin/CompleteRunMutation*`, `AppliesSinceLastClean`); sealed values
(`systemmodel/sealed.go`, `wiring/sealed.go`, `run_sealed_redaction.go`) with the
workspace `secrets: writable_refs` policy; `--only` check scoping
(`acute.TupleScope`, `checks.Partition`, `advisory_count`); the pinned-producer
snapshot path used only by approval replay; `PromoteConvergingToClean*`; and
differential drift, which needed a converge arm.

`internal/acute/session.go` keeps the run identity (`RunSession`,
`NewRunSetID`, `NewMemberRunID`).

Public API added: `systemmodel.RetiredFieldsDoc`. `NewTemplate` and
`DecodeValue` reject retired fields before validation with
`model uses retired field(s): <path> (<reason>); remove them — see
docs/retired-commands.md for replacements`. The schema keeps the retired
shapes (`#PluginPlan`, `#PluginObserve`, `#EweTarget`, `#PluginDef`) only so
such model files compile and can be rejected by name.

## Step 2 — mu-backed observers (`pudl-z87.2`)

Removed `#PluginObserve`, `#EweTarget`, `plugins:`, `run --populate plugin:`
and `--input`, `model populator add/new`, `model show --discover`, the mu
runner, project lock and plugin metadata, drift observations, and `--mu-root`
and `--mu-timeout` (their only consumer was the mu runner). `Populate` holds
only the command arm; `PopulateKind` is `command` or `none`. `runDeps` no
longer carries a subprocess seam.

`pudl model new <name>` scaffolds a `#CommandObserve` arm by default; without
`--populate` it runs a placeholder (`echo []`) with a note to replace it.

The git-inventory example observes with `cat` over its fixture instead of a
Python mu plugin. Run-set integration tests use command arms.

## Step 3 — mu bridge (`pudl-z87.3`)

Removed `pudl mu ingest-manifest`/`ingest-observe`, typed-envelope import, the
mu schema cache (`internal/muschemas`), mu plugin packages, the doctor mu
check, the `schema-refs` repair and `--schema-ref` (it resolved envelope
references against the mu schema cache), plugin schema mappings in ingest, the
unused `toolchain_mappings` setting, and `proc.Output`/`proc.Available`.

Moved `ObserveResult`, the ingest, prepare and route code, snapshot recording
and redaction from `internal/mubridge` to `internal/observe` (package
`observe`; `IngestObserve`, `ObserveIngest`, `ObserveIngestResult`,
`PrepareObservation`, `NewSnapshotID`). `internal/mubridge` is deleted;
`internal/proc` stays (`Command`, `Stopped`, `Cancelled`, `DefaultGrace`).

The Git walkthrough now observes with `pudl run`; one `cp` swaps in the
changed inventory.

## Step 4 — compatibility (`pudl-z87.4`)

- `pudl run report` shows stored fields its report types no longer have under
  "legacy fields" as JSON, including a set member's `mutation_required`.
  `--json` already passes stored reports through unchanged.
- `converging` reads as `unknown` in entry reads, target statuses and the
  `catalog_entry` view; it is no longer a valid write. Stored values are left
  as recorded.
- The `pudl/mu` bootstrap schemas stay, marked legacy; `#ObserveResult` and
  `#ObserveSnapshot` remain in use.
- `run_approvals`, `run_set_approvals` and `item_schemas` keep their tables and
  migrations; nothing writes them. The runs table keeps its legacy columns.
- `cmd/legacy_catalog_test.go` seeds a mutation-era catalog and checks list,
  show, status, run report and doctor.

## Step 5 — examples, docs, tooling (`pudl-z87.5`)

Removed `make test-kick-tires`, the mu smoke tests (host, k3s, kick-the-tires,
cross-model derive) and CI's mu install. `make test-git-walkthrough` also runs
the example observation journey. Regenerated the CLI reference and help
golden. Updated the README, FEATURES, getting started, TESTING, architecture,
concepts, evidence and other live docs, `pudl prime`, `pudl guide` (the `mu`
topic is gone) and the embedded `pudl-core` skill. Convergence-era docs carry a
historical banner. `docs/retired-commands.md` lists every retired command,
flag and model field with its replacement.

## Verification

`CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 make lint`, `make check-skills
check-docs`, `make test-git-walkthrough` and the full smoke suite
(`go test -tags=smoke ./test/smoke/`) pass with no mu installed. Production
code no longer invokes mu; remaining matches of the word in `cmd/` and
`internal/` are legacy names (`pudl/mu.#…` schemas, `mu-observe` snapshot
source) and retirement messages.

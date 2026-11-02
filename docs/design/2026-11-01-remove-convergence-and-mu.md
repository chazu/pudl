# Remove convergence and mu integration

**Date:** 2026-11-01

**Status:** Implemented 2026-11-01 (beads epic `pudl-z87`, steps `pudl-z87.1`
through `pudl-z87.5`); see `implog/2026_11_01_remove_convergence_and_mu.md`.

**Supersedes:** the 2026-10-10 decision to keep convergence as an advanced,
optional workflow ([scope reduction](2026-10-10-scope-reduction-and-improvement-order.md),
"Capabilities to preserve"), and the Mu ownership boundary in
[cross-resource value wiring](2026-07-28-cross-resource-value-wiring.md).

## Decision

PUDL is for SREs who **discover, catalog and govern** cloud infrastructure.
It stops **changing** infrastructure. We remove the convergence workflow and
every integration with mu. Observation runs plain commands that print JSON
(`#CommandObserve`). Governance is checks, desired-state comparison against
the catalog, and reports.

## Why

- **The target user does not use it.** The GCP cataloging session that shaped
  the October work was with "a GCP shop [that] does not use mu"
  ([findings](../gcp-cataloging-workflow-findings.md)). Terraform, mu and
  similar tools already own changes.
- **It is the most complex part of PUDL.** The mutation and mu paths cover
  exact-plan approval and resume, write-ahead mutation intents and receipts,
  apply budgets, sealed inputs and outputs, mutating run sets, mu plan
  canonicalization, plugin metadata, the mu schema cache and manifest ingest.
  That is roughly 6,000 production lines in about 30 files, plus their tests,
  the real-mu acceptance matrix (`make test-kick-tires`), two approval tables,
  and much of the documentation and onboarding.
- **It carries security exposure.** Two of the 2026-11-01 sensitive-data
  findings exist only because of mu paths: drift observations from mu plugins
  store raw plugin output (`pudl-bq5`), and manifest ingest stores non-sealed
  action outputs (`pudl-8b7`).
- **It blocks the simplifications we want.** A single `pudl check` command
  needs no approval lifecycle once nothing mutates.

## What is removed

| Area | Removed | Main locations |
| --- | --- | --- |
| Converge workflow | `pudl run --converge` and its flags (`--only`, `--dry-run`, `--max-iters`, `--max-applies`, `--require-approval`); the converge loop; apply budgets | `cmd/run_converge.go`, `internal/acute/coordinator.go`, `internal/acute/plan.go` |
| Approvals | `pudl run resume`, `pudl run reject`, mutating `run set`, exact-plan approval, write-ahead mutation intents and receipts | `cmd/run_approval_cmd.go`, `cmd/run_set_approval_cmd.go`, `cmd/run_set_mutation.go`, `cmd/run_set_mu_plan.go`, `internal/acute/runset_mutation_plan.go` |
| Sealed values | `sealed_inputs`, `sealed_outputs`, sealed routing and redaction | `internal/systemmodel/sealed.go`, `internal/wiring/sealed.go`, `cmd/run_sealed_redaction.go` |
| mu-backed observers | `#PluginObserve` (including differential drift), `#EweTarget`, the `plugins:` block, `run --populate plugin:<name>`, `pudl model populator add/new` | `cmd/run_drift.go`, `cmd/run_populate.go`, `cmd/model_populator_new.go`, `cmd/mu_runner.go`, `cmd/run_mu_lock.go`, `cmd/mu_plugin_info.go`, `cmd/plugin_metadata.go` |
| Converge arm | `converge: #PluginPlan` | `internal/systemmodel/schema.cue` |
| mu bridge | `pudl mu ingest-manifest`, `pudl mu ingest-observe`, drift observations, typed-envelope import, the mu schema cache | `cmd/mu.go`, `cmd/ingest_*.go`, `cmd/import_envelope.go`, `internal/mubridge/{manifest,plugin,installed_plugin,envelope,drift_observation}.go`, `internal/muschemas/` |
| Tooling | `pudl doctor`'s mu check, `--mu-timeout`, `make test-kick-tires`, CI mu installation, mu-specific smoke tests | `internal/doctor/mu_check.go`, `Makefile`, `test/smoke/` |
| Status vocabulary | `converging`, which only an apply can produce | status code and `catalog_entries.status` writers |

## What is kept and reshaped

- **`#SystemModel`** keeps `name`, `observation`, `populate` (now only
  `#CommandObserve`), `checks`, `relations`, `desired`, `depends_on`,
  `freshness`, `inputs` and `bindings`.
- **Desired-state comparison.** Inventory drift diffs `desired` against
  observed catalog records. It reports differences and changes nothing. It runs
  entirely in PUDL.
- **Observation.** `#CommandObserve` is the only populate arm. The generic
  ingest code now in `internal/mubridge` (`ObserveResult` wrapping,
  `prepare.go`, `route.go`, `ingest.go`, observe snapshots, sensitive-field
  redaction) moves to a neutral package (`internal/observe`), because the
  command arm depends on it.
- **Subprocesses.** `internal/proc` stays: the command arm uses it for
  cancellable runs. Only its mu-specific callers go.
- **Coordination.** Observe-only `run set` stayed at first, together with
  plain value bindings (see [open questions](#open-questions)); both were
  later replaced by `pudl check` and removed. `depends_on` facts and the
  shipped dependency rules stay.
- **Unchanged:** snapshots, checks and verdicts, observation scopes, reports,
  bundles, facts, Datalog, the public library API, and `list --fancy`.

## Compatibility and data preservation

Removal preserves existing evidence. It does not migrate data destructively.

- **Database.** Keep the `run_approvals` and `run_set_approvals` tables and
  their migrations, so existing catalogs open unchanged. Nothing writes to them
  any more. A later, separate migration may drop them.
- **Historical reports.** Stored reports from converge or mutating runs remain
  readable. `pudl run report` renders fields it no longer understands as legacy
  JSON instead of failing.
- **Catalog entries.** Existing entries of mu types (manifests, actions,
  `pudl/mu.#DriftObservation`) stay listable and showable. The bootstrap
  `pudl/mu` schemas are kept, marked legacy, so those entries still validate.
- **Status.** `converging` rows read as `unknown`, so a missing observation is
  never shown as clean.
- **Model files.** A model that uses a removed field (`converge`, `plugins`,
  `sealed_inputs`, `sealed_outputs`, `#PluginObserve`, `#EweTarget`) fails to
  load. The error names the field and links to the migration notes in
  `docs/retired-commands.md`. Nothing is silently ignored.
- **Retired commands.** The removed commands and flags are listed in
  `docs/retired-commands.md` with replacements. For mu observers, the
  replacement is a `#CommandObserve` run of the same program.

## Effect on open issues

- `pudl-bq5` (drift observations skip redaction) and `pudl-8b7` (manifest
  outputs stored as-is) are resolved by deleting their code paths. Stored
  plaintext from those paths remains a scrub concern for `pudl-m10`.
- `pudl-o5i` (desired state on sensitive paths) still applies: desired values
  appear in inventory drift reports.
- `pudl-udi`, `pudl-oa4`, `pudl-m10` and `pudl-167` are unaffected.

## Delivery

Each step is one or more commits that leave the build and tests green. The
beads IDs are `pudl-z87.1` to `pudl-z87.5`, in order. Follow-on work is in
the epics `pudl-4fx` (simplify the everyday command surface) and `pudl-xf5`
(governance for SRE cloud estates).

1. **Remove convergence:** the converge arm and flags, the loop, approvals,
   mutating run sets, mutation intents and receipts, apply budgets, sealed
   values, and the `converging` status. Add load errors for the removed fields.
2. **Remove mu-backed observation:** `#PluginObserve`, `#EweTarget`, `plugins:`,
   differential drift and drift observations, `run --populate plugin:`,
   populator commands, and the mu runner, lock and plugin metadata. `model new`
   scaffolds the command arm by default.
3. **Remove the mu bridge:** the `pudl mu` commands, envelope import, the mu
   schema cache, the doctor mu check and `--mu-timeout`. Move the generic
   ingest code to `internal/observe`.
4. **Compatibility:** rendering of legacy reports, legacy `pudl/mu` schemas,
   mapping `converging` to `unknown`, and regression tests against a catalog
   that contains mutation-era data.
5. **Examples, docs and tooling:** convert `examples/git-inventory` and the Git
   walkthrough to `#CommandObserve`; remove `make test-kick-tires` and CI's mu
   installation; regenerate the CLI reference; update the README, the getting
   started guide, `guide`/`prime` and the embedded skills.

Implementation logs follow the repository convention (`implog/`, `docs/plan.md`).

## Acceptance

- No production code invokes `mu` or reads mu artifacts. `grep -rw mu` across
  `cmd/` and `internal/` matches only legacy-schema and compatibility code.
- `CGO_ENABLED=0 go test ./...`, `make test-race`, `make lint`,
  `make check-skills check-docs` and `make test-git-walkthrough` pass, with no mu
  installed.
- The GCP fixture workflow (discovery, checks, correction, snapshot comparison,
  history) passes unchanged.
- A catalog created before the removal, containing converge reports, approvals,
  manifests and drift observations, opens, lists, shows and reports without
  errors.

## Open questions

1. **Observe-only `run set` and value bindings.** Without mu, bindings feed
   `#CommandObserve` arguments from a producer's observation. That is useful
   but uncommon for inventory work. **Recommendation:** decide during the
   `pudl check` unification. Keep them if a real use appears; otherwise remove
   `run set`, `bindings` and `internal/wiring`.

   **Resolved (2026-11-01, `pudl-4fx.1`): removed.** No shipped example,
   documented workflow or bundled skill relied on bindings; their only users
   were their own tests and the repository's mu kick-the-tires fixtures, which
   had already stopped loading when `plugins` was retired. `pudl check
   MODEL...` replaced `run` and `run set`; it orders named models by
   `depends_on` only, so the number of models named never changes how values
   are resolved. `internal/wiring`, template `inputs`/`bindings`, the
   `@pudl(binding=...)` classification, the producer-snapshot accessors and
   `--max-observation-age` are gone; models that declare `inputs` or
   `bindings` fail to load naming the field. See
   [`implog/2026_11_01_pudl_check.md`](../../implog/2026_11_01_pudl_check.md).
2. **Typed-envelope import.** The envelope format was designed for mu plugins,
   but any producer could use it. This document removes it with the mu bridge.
   If a non-mu producer needs to hand over data with its CUE definitions, it
   can come back as a plain import feature.
3. **Dropping legacy tables.** Decide after one release whether to migrate the
   approval tables and legacy schemas away.

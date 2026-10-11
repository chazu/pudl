# PUDL scope reduction and improvement order

**Date:** 2026-10-10

**Status:** Implementation in progress. The user approved this sequence on
2026-10-10 with `list --fancy` explicitly retained. Delivery evidence is recorded
below as each step completes.

**Review baseline:** `f9be415`.

PUDL should help humans and coding agents understand systems through retained,
inspectable evidence: what exists, what changed, what violates expectations,
and how recently and completely those conclusions were checked. Its local
storage, history, and verification machinery are worth keeping. The next phase
should reduce implicit behavior and the amount of internal knowledge required
to reach a trustworthy answer.

This proposal follows the [September UX assessment](2026-09-30-ux-simplification-report.md)
and the [October GCP workflow changes](2026-10-08-gcp-cataloging-ux.md).
It prioritizes demonstrated correctness gaps before broader interface work.

## Recommended removals

| Removal                                                                           | Reason and retained capability                                                                                                                                            | Cost or condition                                                                                                                                                                             | Delivery step |
|-----------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------|
| `pudl setup` and shell-profile management                                         | Shell detection, startup-file editing, backup, aliases, and uninstall behavior add ownership unrelated to evidence. Keep `pudl completion` and document its installation. | Publish manual cleanup instructions for previously installed snippets; do not edit users' profiles during an upgrade.                                                                         | 1             |
| `pudl model deps --derive` and value-equality dependency guessing                 | Coincidental strings can become dependency edges. Keep declared dependencies and dependencies established by value bindings.                                              | Authors must declare relationships. Retire only facts owned by this heuristic; preserve declared, binding, and user-authored facts and historical evidence.                                   | 1             |
| Implicit global schemas, rules, models, and definitions inside project workspaces | A repository's meaning should be reproducible without its operator's home directory. Keep explicitly selected global mode and explicitly declared shared dependencies.    | Existing workspaces need a dependency inventory and migration path. Preserve bundled standard schemas and inspect the effective dependency set before changing resolution.                    | 5             |
| Silent schema fallback after an explicit schema request                           | An explicit type request should either validate or fail. Keep best-effort inference for exploratory, untyped imports.                                                     | Some imports currently accepted through fallback will fail. Offer an explicit, visibly reported permissive choice where needed.                                                               | 4             |
| `schema status/commit/log`, `schema edit`, and `module add/tidy/list/info`        | These largely wrap Git, an editor, and CUE. Keep schema generation, validation, inference, and dependency resolution that have PUDL-specific meaning.                     | Provide exact paths and equivalent native commands first. This revisits the September decision to retain these helpers.                                                                       | 8             |

The retired heuristic dependency implementation used approximate value matching. The [workspace policy](../../internal/workspace/policy.go)
currently includes global search paths even for local workspaces. The retired shell integration installed navigation helpers centered on
the global schema directory. These are concrete simplification opportunities.

Two further reductions concern the normal workflow rather than capability
deletion. Reinference, reclassification, and reprojection should become
diagnosable maintenance operations behind a coherent repair experience;
explicit recovery controls remain available. Onboarding should teach resources,
observations, expectations, findings, and evidence, introducing ACUTE, EDB,
Ewe, and sealed routing only when the task requires them.

## Capabilities to preserve

**Decision 2026-10-10:** Keep `list --fancy`, its interactive interface, and its
required dependencies. Its removal is outside this implementation scope.

Keep the single binary, SQLite catalog, local workspace storage, content and
resource identities, temporal fact store, Datalog, provenance, retained
snapshots, faithful exports, and portable evidence bundles. Preserve generic
fact APIs for independent consumers; the bundled agent-memory application has
already been removed.

Keep convergence as an advanced, optional workflow. PUDL owns evidence,
expectations, bounded coordination, and interpretation; Mu owns convergence
actions and sealed provider handling. Plain-command observation remains useful
without Mu. Preserve exact-set membership, approved-plan validation, mutation
receipts, apply budgets, and uncertain-outcome handling throughout the work.

## Findings that determine priority

The review built the baseline source and exercised temporary local workspaces.
These are CLI observations, not live-cloud or full-suite qualification:

- A fail-severity `expect: "empty"` check whose rule referenced a nonexistent
  relation returned `passed: true`, `ok: true`, and exit 0, including with
  `--detailed-exitcode`. A warning appeared only on stderr. See
  [check evaluation](../../cmd/run_checks.go) and
  [projection diagnostics](../../cmd/projection_sync.go).
- A command observer emitting valid JSON `[]` failed with
  `prepare observe results: target models/review-observe:populate records is not an array`.
  The [command adapter](../../cmd/run_command_populate.go) accumulates records in
  a nil slice, which is marshaled as `null` when nothing was observed.
- After observing resource A, a later successful observation containing only B
  left both resources in current projected facts. A check in the second run saw
  both. This matches the documented [projection limitation](../projection.md):
  missing resources are not treated as deletions, and projected facts have no
  run ID.
- A catalog replay with a real mismatch returned `ok: true` and
  `completion_status: "succeeded"` alongside `drift.clean: false` and
  `drift.verified: false`. Detailed exit codes correctly returned 2. The
  dimensions are legitimate, but the presentation requires too much interpretation.
- A missing-model request with `--json` produced a textual error without a
  structured result.

The first two are defects to fix. The third needs an explicit evidence-scope
contract: latest-known state across resources and one complete inventory are
different views. The last two motivate a clearer result interface.

## Delivery order

Each numbered step should leave a usable release boundary. Start with the small
removal batch, then prioritize correctness over completing every retirement.
Do not delay the verdict fixes for wrapper removal or a larger refactor.

### Step 1 Remove shell management and guessed dependencies

Remove `setup` and `model deps --derive`, their implementation, and obsolete
help, examples, and tests. Retain completion and authoritative dependency
inspection. Provide manual shell-snippet cleanup instructions and a narrowly
scoped migration for heuristic-owned dependency assertions, preserving history.

**Acceptance:** removed commands and flags fail clearly; completion still
works; declared and binding dependencies produce the same exact run-set order;
the migration leaves user assertions and unrelated provenance untouched.

### Step 2 Fix empty observations and unsupported passing checks

Accept zero-record command observations and retain their snapshots. Distinguish
a legitimately empty declared relation from an unknown relation or a broken
projection. Make material evidence diagnostics part of the persisted JSON report.

Introduce explicit check outcomes: pass, fail, unknown, and evaluation error.
An unknown outcome means PUDL cannot establish the check's claim; an evaluation
error means it could not execute the check. A missing relation, invalid required
projection, or failed required synchronization must not become a pass merely
because the query returned zero rows. Consider only dependencies relevant to
the check so an unrelated broken schema does not disable the whole workspace.

Define machine exit behavior and report-version compatibility together.
Unknown/error outcomes must not signal a successful verification; retain the
documented distinction between a completed operation and a conformity result.

**Acceptance:** regressions cover valid empty output, missing relations,
invalid projections, failed synchronization, and genuinely empty valid inputs.
JSON, text, retained reports, and detailed exit codes agree. No mutation is
needed to exercise these cases.

### Step 3 Unify identity and define observation completeness

Use a shared resource-identity contract across import, drift, and projection,
including nested and quoted field paths. The current
[inventory matcher](../../internal/acute/inventory_diff.go) reads identity
fields as top-level keys and uses separate fallbacks. Preserve existing stable
identities where possible; any changed identity requires an explicit migration.

Define an observation's source, scope, time, and completeness. A complete
inventory may establish absence within its declared scope after successful
collection. A partial observation, failed page, or failed fan-out member cannot.
Do not infer completeness merely from command exit 0 or a JSON array.

Let checks select a snapshot or an explicit catalog scope and freshness policy.
Keep latest-known facts as a distinct, labeled view. For cross-source checks,
retain the selected evidence set and its freshness rather than implying that
all sources were observed simultaneously. End current memberships or assertions
only under the appropriate complete-inventory contract; retain historical data.

**Acceptance:** tests cover complete and partial observations, zero resources,
disappearance, reappearance, incomplete pagination, failed collection,
overlapping sources, unrelated scopes, and nested identities. A resource absent
from a complete observation stops satisfying checks about that inventory without
disappearing from history. A partial observation cannot erase it.

### Step 4 Enforce explicit schema requests

Make explicit import and command-observer schema requests strict. Validate before
publishing catalog/projection changes for the defined import unit. Document that
unit and any supported partial-success behavior. Preview must predict actual
acceptance, and identical-content dedup must not bypass a new validation request.

Retain an explicit permissive mode for exploratory use where justified. Report
requested and assigned schemas, mismatches, and skipped records structurally.
Preserve sensitive-field handling even when validation fails or permissive
fallback is requested.

**Acceptance:** valid imports work; mismatches do not silently change type;
preview and execution agree; collection, dedup, redaction, and projection paths
obey the same contract. Document the behavior change with migration examples.

### Step 5 Make project definitions reproducible

Inventory effective local and global dependencies, then support explicit shared
definitions through declared, reproducible dependencies or vendored files.
Remove ambient global fallback from project resolution once those dependencies
are available. Preserve explicitly selected global mode.

Unify resolution for the CLI and public library. Show where a schema, rule, or
model came from. Avoid automatic migration that copies unrelated home-directory
configuration or silently changes the meaning of existing rules.

**Acceptance:** a migrated project produces equivalent classification and
check results under a clean home directory and one containing conflicting global
definitions. Global mode remains usable. Missing dependencies fail with the
specific definition and a concrete repair action.

### Step 6 Make reports coherent for humans and agents

Expose execution outcome, conformity, checks, evidence freshness/completeness,
scope, and verification in both standalone and aggregate reports. Extend
[run-set member results](../../internal/acute/runset_report.go) so callers need
not retrieve every member report to establish whether the set is verified.

Lead human output with the result and its evidence qualification. Version the
machine contract, return structured errors for failures before a run exists,
and include material diagnostics and actionable inspection/repair commands.
Add useful phase progress without mixing it into final JSON output. Document
complete enumeration and pagination for list/query clients.

Extract the application orchestration touched by this work from Cobra handlers
into a narrow service accepting explicit workspace, request, and dependencies.
Return reports and progress events; keep presentation in the CLI. Preserve the
existing coordinator and test seams, and avoid introducing a general framework.

**Acceptance:** a human can interpret a set from one report; an agent can make
the same decision without scraping stderr or guessing the meaning of `ok`.
Failure, replay, stale evidence, pending approval, and uncertain mutation have
distinct, tested representations. Retained older reports remain readable.

### Step 7 Deliver a complete daily investigation workflow

Use GCP network hygiene as the first real workflow, based on the existing
[cataloging feedback](../gcp-cataloging-workflow-findings.md). Package collection,
schemas, checks, fixture data, and evidence inspection together. Retain the
credential-free Git example for onboarding and automated acceptance.

Support direct field inspection and simple filtering before requiring users to
author projections and Datalog rules. Reuse existing machinery where possible;
keep explicit rules for richer joins and recursion. Let a useful investigation
become a saved check with minimal repeated configuration. Make observation
comparison and resource history easy to discover through existing commands.

Present required reinference, reclassification, or reprojection through clear
diagnosis and a reviewable repair action. Update `prime`, targeted help,
scaffolds, and examples around the completed workflow. Introduce Mu only when
the user chooses a workflow that requires it.

**Acceptance:** a human and an agent can each start in an unfamiliar workspace,
collect or load evidence, identify a problem, explain its scope and freshness,
save a check, and verify a correction. Record commands, custom glue, mistakes,
and time to a justified answer against the current workflow. Fixtures test the
journey hermetically; live GCP acceptance is recorded separately.

### Step 8 Retire wrappers and retain the interactive list

Once effective paths and the ordinary inspection workflow are clear, retire the
Git/editor/CUE wrappers with exact native-command replacements. Preserve schema
version control and CUE dependency support. Treat the command removal as an
announced compatibility change, not an incidental cleanup.

Keep `list --fancy` and its existing behavior. Wrapper retirement must preserve
the interactive list and its dependencies.

**Acceptance:** authoring, dependency maintenance, and schema history remain
straightforward using documented paths and native tools. Examples and agent
instructions contain no retired commands. Dependency cleanup reflects actual
remaining imports; the interactive list remains available.

## Validation and scope control

Use focused regression and contract tests for each behavioral change. For
integrated releases, run the repository's documented checks, including
`mise exec -- make test-race`, generated docs/skills checks, and the Git
walkthrough. Run real-Mu acceptance when run coordination, approvals, or bridge
behavior changes. Treat live-cloud qualification as separate evidence.

Record each removal's migration and each step's acceptance result before calling
it delivered. Update this proposal's status as work lands. Existing authored
data, facts, schemas, shell profiles, and historical reports must survive
retirement of the commands that previously managed them.

Keep dashboards, background services, new agent transports, additional format
parsers, and broad command-hierarchy redesign outside this sequence. The success
criterion is fewer concepts and fewer opportunities for an incorrect conclusion
between an observation and an answer that a human or agent can justify.

## Delivery record

- Step 1 implemented: removed shell-profile management and heuristic dependency
  derivation, with migration 24 preserving history and unrelated sources. Focused
  migration and run-set tests passed. `list --fancy` remains supported.
- Step 2 implemented: empty command observations are retained, and required
  evidence failures produce unknown/error check outcomes with durable structured
  diagnostics. Full cmd/projection/acute tests and focused health regressions
  passed. New standalone reports are version 2; old reports remain readable.
- Step 3 implemented: nested identity extraction is shared; migration 25 records
  explicit scope, completeness, and schema coverage. Checks can select snapshots
  with an age policy, evaluated in an isolated disk-backed view. Latest-known
  facts remain historical knowledge; selected complete inventories establish
  scoped absence without deleting them. Cmd, database, acute, systemmodel,
  bridge, and bundle tests passed, including disappearance/reappearance,
  partial/empty/failed collection, freshness, and owner-ambiguity regressions.

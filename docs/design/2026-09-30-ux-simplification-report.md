# PUDL UX simplification design report

**Date:** 2026-09-30

**Status:** Partially implemented. Command consolidation is delivered; broader scope and workflow proposals remain open.

**Scope:** Product purpose, CLI, documentation, authoring, and workflows for humans and agents.

PUDL should center on one purpose: **record what a system looks like, compare it
with what we expect, and explain the result with retained evidence. Mu performs
the actions.** Imports, schemas, snapshots, facts, and queries support that
purpose. The agent self-improvement application is a separate product and
should leave the core UX.

This report recommends reducing product scope, consolidating operational
commands, and making results directly interpretable. The first delivery should
improve aggregate reports and machine output. Subsequent work should retire
unnecessary features and unify checking, applying, reporting, and approvals.

## Implemented command consolidation

The first implementation consolidates overlapping commands while retaining the
schema/Git and module/CUE helpers at the user's request. The explicitly
registered top-level command count is reduced from 33 to 28.

- Local and global initialization use `init` and `init --global`.
- `schema list` includes schema catalog metadata.
- `model show` replaces `model describe`; optional `--discover` includes Mu
  capabilities while ordinary inspection remains independent of Mu.
- `doctor` combines health, schema validation, and inference stability checks.
- Exact sets use `run set`. Standalone and set reports and approval decisions
  share `run report/resume/reject`, dispatched by stored operation identity.
- `list` shows the active catalog without an implicit origin filter.

This consolidation preserves the existing distinction between standalone
recorded-input reuse and exact-set producer selection. It also preserves
standalone request-level approvals and set exact-plan approvals. The proposed
`check`/`apply` interface, a universal result contract, saved-input shorthand,
memory extraction, and stronger standalone approvals remain proposals.

See [command migration](../../README.md#command-consolidation) and the
[implementation log](../../implog/2026_09_30_command_consolidation.md).
The analysis below describes the review baseline and broader recommendations.

## Review scope and evidence

Three independent reviews examined project history and purpose, human CLI
workflows, and agent workflows. Their findings were checked against current
source, documentation, and executable help at commit `b0b035f`.

The source registers 33 top-level commands and 76 leaf command paths, excluding
help, generated completion, and aliases. Some parent commands also execute.
The command count illustrates the breadth of the interface; the more significant
problem is how many independent concepts users must understand to finish a task.

This was a source and history review, not a new live infrastructure acceptance
run. The [Batcave operator feedback](../mu-pudl-batcave-user-experience.md)
provides experience from an actual integration and identifies which friction
belongs to that adapter. No usage telemetry establishes that optional features
are unused. Recommendations to extract or retire them are product judgments.

## How the product grew

| Period | Product direction | Historical evidence |
| --- | --- | --- |
| August 2025 to February 2026 | Personal cloud data lake, with imports, inference, collections, identity, and schema management | `7504ea9` establishes the data-lake direction; `04da33d` adds schema generation; `79b362b` adds resource identity |
| March 2026 | Infrastructure automation platform, with methods, sockets, workflows, and embedded execution | `ccac41d` expands the roadmap; `f79297b` removes execution; `61de8b3` restores it; `c355145` removes it again |
| April to June 2026 | Bitemporal knowledge database, Datalog, and an agent-memory application | `89857e6` introduces facts; `e2c845b` adds Datalog; June work adds memory, curation, and harness hooks |
| June to August 2026 | System models, live observation, convergence, cross-model wiring, and approvals | `5ca6725` establishes the model loop; `0e7a981` adds value wiring; `99a6f16` completes the sealed run-set bridge |
| September 2026 | More reliable onboarding, schema discovery, and operational feedback | `13687ca` and `b6390a9` deliver the installed walkthrough; `bfa9d55` improves schema references |

The March expansion explicitly said, “Nothing is removed — scope grows.” The
execution ownership changes that followed eventually established a useful
division: PUDL owns knowledge, comparison, and bounded coordination; Mu owns
external execution and secrets.

The current CLI still presents several generations of features together.
Historical documentation also leaks into current guidance:
[VISION.md](../VISION.md) advertises removed type patterns and
`schema generate-type`. The [vestige sweep](../vestige-sweep.md) records their
removal. Such contradictions make agents select nonexistent commands and make
humans doubt which documentation describes the installed tool.

## Recommended product scope

The core product should answer four questions:

1. What did we observe, and when?
2. Does it match our expectations and pass our checks?
3. What changes would reconcile it?
4. What evidence supports the result after execution?

Keep typed import, CUE inference and validation, resource identity, temporal
history, retained observations, desired-state comparison, model checks, and
reports. Keep bounded model coordination where a consumer must use a producer's
selected observation or where PUDL must elaborate values between executions.

Facts and Datalog remain useful implementation and library capabilities.
Dependencies and checks use them, and [the public library](../library-api.md)
supports independent consumers. Ordinary model users should not need to learn
arbitrary relations, two temporal axes, or fact lifecycle operations first.

Extract the agent-memory application from the core CLI: memory context and
cycles, reflection-agent orchestration, maturity and curation policies, and
harness hooks. These are live capabilities, not dead code. Their retirement
needs an explicit migration decision that preserves existing data. A separate
consumer can use the fact-store library if those workflows remain valuable.
See [memory](../../cmd/memory.go), [hooks](../../cmd/hooks.go), and
[curation](../../cmd/facts_curate.go).

## Proposed everyday command surface

The following table sketches a proposed combined interface. Some command names
are reused; the new commands and semantics are not implemented.

| Proposed command | Purpose |
| --- | --- |
| `pudl init` | Create or repair the local workspace |
| `pudl model list` | Discover available models and their last results |
| `pudl check MODEL...` | Observe and evaluate the explicitly selected models |
| `pudl apply MODEL...` | Reconcile through Mu and verify the outcome |
| `pudl report [ID]` | Inspect any operation, whether it involved one model or several |
| `pudl import FILE` | Retain external data or saved observations |
| `pudl list` / `pudl show ID` | Browse retained evidence |

Schema and model authoring, expert queries, configuration, and diagnostics remain
secondary capabilities. Command count alone is not the target: users should
have one obvious path for a task, and removing features should reduce the code
and documentation that must be maintained.

### Consolidate operations and approvals

Replace ordinary `run` and `run-set` usage with a shared check operation, and
replace their convergence modes with a shared apply operation. Use one report
surface and one approval lifecycle for both single-model and multiple-model
operations. Display model names consistently; internal target keys such as
`//models/name` belong in detailed evidence.

This requires a deliberate policy decision. Singleton runs can currently bind
recorded producer snapshots, while exact run-sets require binding producers
inside the explicitly named set. The unified interface must make use of
recorded upstream evidence explicit and inspectable. Argument count should not
silently select a different policy. PUDL must continue executing only the named
models and must not discover and run missing producers automatically.

Approval also has different implementations today. A single-run resume restores
the request and re-enters execution, while run-set resume rebuilds the plan and
compares its canonical digest. The shared operation should use the stronger
exact-plan semantics. Unification cannot be accomplished by simply aliasing
the existing commands. See [single-run approval](../../cmd/run_approval_cmd.go)
and [run-set approval](../../cmd/run_set_approval_cmd.go).

### Simplify saved observation checks

The [introductory walkthrough](../getting-started.md) currently uses:

```text
pudl mu ingest-observe --path FILE --origin NAME
pudl run MODEL --from-catalog --catalog-scope NAME
```

It must explain that Mu is unnecessary, what an origin selects, and why stored
replay differs from live observation. A proposed direct path is:

```text
pudl check MODEL --input FILE
```

The operation would ingest a supported saved observation, pin its evidence,
evaluate it, and return a report. It must identify the result as a check of
saved evidence, preserve observation timestamps and provenance, and avoid
claiming a fresh live verification. Ordinary imported data must not become a
trusted observation merely because its shape looks plausible.

Explicit snapshot selection should also replace the paired
`--from-catalog --catalog-scope` flags. The existing scope requirement prevents
comparisons against unrelated records and must survive the simpler interface.

## Make results directly interpretable

Aggregate result clarity is the first priority. The Batcave feedback records
fetching twelve individual reports after a successful twelve-member run-set to
establish cleanliness and verification. The current
[aggregate report structure](../../internal/acute/runset_report.go) contains
member execution results without their drift and verification summaries.

A proposed human view is:

```text
12 models checked
10 match expectations · 1 drifted · 1 could not be verified

MODEL       STATE       EVIDENCE       PROBLEM
balthazar   matches     live, 20s ago   —
caspar      drifted     live, 18s ago   governor disabled
melchior    unknown     unavailable    observation failed
```

This example illustrates presentation, not a recorded fleet result. Checks,
bindings, hashes, and receipts remain available in the detailed report.

The structured representation should expose separate fields for operation
completion, resource conformity, check results, evidence source and age,
evaluated scope, mutation and verification, unresolved problems, and pending
approval. A short headline can summarize these dimensions, but must not erase
them.

The tutorial currently explains that `ok: true` can coexist with drift, and
that replay has `drift.verified: false`. Those distinctions are valid. Requiring
every caller to reconstruct the operational answer from several reports is the
UX problem. The same applies to an unknown status caused by a lost receipt:
show the reason directly rather than requiring a second investigation.

Long-running operations should emit phase and progress updates through a
structured channel, with a consistent human presentation. Mu and domain plugins
need to participate; the Batcave adapter currently hides much of its work inside
one action. Progress output must remain separate from the final JSON result.

## Remove duplication and unnecessary wrappers

| Current surface | Recommendation |
| --- | --- |
| `schema status/commit/log/edit` | Use Git and the editor directly |
| `module tidy/list/info` | Use CUE's tooling directly; review whether module installation adds PUDL-specific value |
| `catalog` and `schema list` | One schema listing with optional detail |
| `model show` and `model describe` | One inspection command with text and JSON renderers |
| `run report` and `run-set report` | One operation report command |
| Separate resume and reject trees | One approval lifecycle using exact-plan semantics |
| `init` and `repo init` | Local initialization by default, with explicit global selection |
| `validate`, `verify`, and `doctor` | One diagnostic entry point with clearly named checks |
| Migration, reinference, reclassification, and pruning | Secondary maintenance capabilities with explicit mutation intent |
| `setup` shell navigation helpers | Retire convenience wrappers that add little domain value |

Retain scaffolding and inference where they save meaningful PUDL-specific work.
Avoid silently reclassifying retained data during reads. Moving every command
under an advanced group would reduce visual clutter while preserving the
underlying maintenance burden; removal and consolidation are necessary too.

## Make workspace behavior predictable

Use a local workspace by default, require explicit selection for global use,
and preserve repository isolation. Discovery and help must not initialize
global state. Install agent-specific integrations only when requested; current
[repository initialization](../../internal/repo/init.go) installs Claude skills
as part of creating a data workspace.

Bare `list` should show the active catalog unless the caller explicitly requests
an origin filter. Today [listing](../../cmd/list.go) defaults its origin filter
to the workspace name, so imports with a custom origin can disappear from the
default view. `--all-workspaces` only removes that filter inside the same
catalog; its name suggests a broader operation than it performs.

Workspace selection, source provenance, and evidence selection are distinct
concepts. Present them when they affect the answer, rather than making users
learn their storage relationships before they can list what they imported.

## Give agents a dependable output contract

Humans and agents should use the same commands and semantics. JSON is an
alternate representation of the same result.

The current global `--json` promise is inconsistent. For example,
[facts observe](../../cmd/observe.go) prints prose before JSON and returns a
text-only response for duplicates. Several other commands ignore the flag or
bind separate local JSON flags.

For the retained surface:

- Return one versioned result document on stdout in JSON mode.
- Send diagnostics and progress to stderr.
- Return stable error codes and actionable context.
- Preserve consistent result shapes across first execution and replay.
- Define exit behavior for detected problems separately from inability to
  perform the operation.
- Make discovery free of initialization side effects.

The exact schema and exit-code mapping remain design decisions. Existing
[run reports](../../cmd/run_report.go) supply useful building blocks, including
report versioning, completion, checks, errors, and binding evidence.

Shrink prime, guides, and installed skills to one short agent entry point:
purpose, workspace, safe default, core workflow, output contract, and targeted
help. Generate the command reference from the implementation and visibly mark
historical designs as historical. Preserve useful task guides without
maintaining several independent descriptions of the command tree.

## Simplify authoring within CUE

The [SystemModel schema](../../internal/systemmodel/schema.cue) requires only a
name and a populate arm. The difficulty is connecting resource schemas,
observers, actions, credentials, and generated configuration.

Use reusable CUE definitions and shared declarations to reduce repetition.
Scaffolding should return the files to edit and make the common observer path
small. Repeated credential declarations and brittle insertion of CUE attributes
deserve particular attention, as recorded in the Batcave feedback.

Keep the existing model and Mu plugin contracts as the foundation. Generated
configuration volume alone is not a useful measure of complexity; repeated
authored declarations and required regeneration are the friction to reduce.

## Guarantees simplification must preserve

- Execute only explicitly selected models, with deterministic dependency order.
- Pin producer evidence and preserve workspace, source, and snapshot provenance.
- Distinguish saved replay from fresh live observation.
- Preserve exact-plan validation and mandatory approval for sealed-output plans.
- Keep secret values inside Mu's provider and execution path.
- Retain receipts and report uncertain mutation outcomes as needing verification.
- Preserve temporal history, resource identity, and existing stored evidence.
- Keep scoped success distinct from proof that the entire model is clean.

PUDL should retain narrow coordination where observation selection and CUE
elaboration must occur between executions. Mu continues to own actions and
secrets. These guarantees explain why some internal complexity earns its place.

## Recommended delivery sequence

### Delivery one results and machine output

Expose conformity, check failures, verification, evidence age, scope, and
unresolved problems in the aggregate report. Use one result representation for
text and JSON. Standardize machine output and discovery behavior across the
commands that will remain.

Acceptance: a caller can determine each member's operational result from one
aggregate response, including partial failure, saved replay, and uncertain
mutation. JSON output can be parsed without stripping prose or making extra
requests for basic verdicts.

### Delivery two scope reduction and workspace behavior

Decide the migration or retirement path for the memory application, remove
unnecessary tool wrappers, consolidate inspections, simplify initialization,
and remove implicit origin filtering. Update current guidance and identify
historical documents clearly. Preserve existing data and communicate deliberate
command removals; any transitional aliases should have a bounded lifetime.

Acceptance: workspace creation has one ordinary path, imported data is visible
in the active catalog, and core onboarding does not teach memory curation,
harness hooks, or Git wrappers.

### Delivery three unified operations and authoring

Resolve the upstream evidence policy and unify checking, applying, reporting,
and approvals. Add a direct saved-observation path. Reduce repeated model and
credential declarations within existing CUE and plugin contracts.

Acceptance: the same task vocabulary handles one model or an explicit set,
approval refers to the exact execution plan, and saved input is clearly
distinguished from live verification. Exercise the real Mu bridge when changing
execution behavior; command aliases and mocked plans alone cannot prove these
guarantees.

## Decisions needed before implementation

The recommendations do not yet settle the exact result schema, exit-code
mapping, approval syntax, or upstream evidence policy. Extraction of memory
also requires deciding whether to maintain a separate consumer and how existing
integrations migrate. These are implementation design questions, not reasons
to keep the entire current surface indefinitely.

The governing acceptance criterion is practical: **a human or agent can discover
a model, check it, understand the result, and inspect its evidence without first
learning PUDL's storage architecture.**

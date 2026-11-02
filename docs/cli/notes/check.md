**A check observes and reports; it never changes anything.** For each named
model it runs `populate`, compares `desired` with what was observed, evaluates
the checks and records the result. [Governance](governance.md) is the guide to
checks, findings, policies, waivers, changes over time, owners and CI use.
[Observation](observation-scope.md) covers collectors, fan-out and evidence
selection.

**Exactly the named models.** `pudl check a b c` runs `a`, `b` and `c` and
nothing else. A model runs after every named model it lists in `depends_on`;
dependencies that are not named add no ordering and are never run. When a
named producer does not succeed, its dependents are recorded `blocked`;
independent models continue. The report records the order and the edges
(`ordered_members`, `edges`, `plan_digest`).

**One report.** Each invocation records one report: an aggregate `summary`
(execution, conformity, checks, verification, finding counts, evidence), and
one entry per model under `members` with its result, run ID, summary and full
`report`. Under `--json` that report is the only stdout document. Human output
shows each model's findings as it concludes, then the summary. Read it again
with [`pudl report`](#pudl-report).

**Exit status.** Without `--detailed-exitcode`, a check exits 0 when every
model completed, including when it found drift, and nonzero on a failing
fail-severity check or any error. With it: 0 clean, 2 findings, 1 error. See
[run in CI](governance.md#run-in-ci) for the full rules and `--fail-on new`.

**Checks-only models.** A model without `populate` evaluates its checks over
the catalog as it is, after syncing projected facts. It cannot declare
`desired`.

**Catalog replay is not an observation.** `--from-catalog` compares `desired`
with records already in the catalog, selected by `--catalog-scope` (a
snapshot ID or an import origin). It names exactly one model and never
updates the model's status.

**Interrupting.** The first Ctrl-C (SIGINT or SIGTERM) sends SIGTERM to the
running command and gives it 10 seconds, removes temporary workspaces, and
records the current and remaining models `cancelled`. A second Ctrl-C exits
at once with status 130. A populate `timeout` is recorded as `failed`.

**`--check-upstream`** warns when a model this one transitively depends on is
recorded `drifted` or `failed`; see
[cross-model dependencies](cross-model-dependencies.md).

A model that declares a retired field (`converge`, `plugins`, `inputs`,
`bindings`, sealed values) fails to load with an error naming it; see
[retired commands](retired-commands.md).

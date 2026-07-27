# One workspace policy, shared by CLI and library (Recommendation 5)

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-workspace-policy.md`
**Closes:** Recommendation 5 from `docs/architecture-improvement-report.md`

## What was wrong

Workspace precedence was implemented but re-implemented per call site. The
"global then repo" rule ordering was assembled independently in four places
(`pudl query`, model checks, `pkg/factstore`, and the rule-write target), and
`modelSearchDirs` called `workspace.Discover(".")` a *second* time rather than
reading the discovery the CLI had already done at startup. The report's success
measure — "workspace resolution is identical across CLI and library APIs" — held
by coincidence, not by construction.

`workspace.Context` carried four fields, two of which nobody read, and could not
be extended without touching every call site.

## What changed

`workspace.Policy` replaces `Context`: one value, resolved once in
`PersistentPreRunE`, carrying schema, definition, rule and model search paths,
the effective origin, and the mode. `Resolve(cwd, globalDir)` is the single
assembly point, and `pkg/factstore.DiscoverWorkspace` now delegates to it.

Orderings are preserved exactly, including one that looks wrong.
`RulePathsForModel` yields `[global, modelDir, repo]`, so a repo workspace's
rules shadow a model's own — arguably backwards, since a model's `rules/` is
more specific than the repo it sits in, but changing which rules shadow which
changes what a check evaluates. Flagged in the design rather than quietly fixed.

Two shapes did not fit a flat list, and are methods instead of fields:

- `CatalogScope()` is an alias for `EffectiveOrigin`, with the reason stated: the
  only catalog scope a *workspace* implies is the origin its records are written
  under. A run's `--catalog-scope` is per-invocation, and Defect 3 made it
  explicitly required rather than inferred.
- `PopulatorPathsFor(ownerRoot, modelDir)` is owner-relative, not
  repo-then-global: a populator resolves against the pudl root that owns the
  model, so a globally-registered model cannot silently pick up a repo's
  populator of the same name.

## Design correction found during implementation

The design had the policy filter `RuleSearchPaths` to existing directories,
unifying the one call site that filtered with the three that did not. A
`factstore` test caught it: `RulePaths` is documented as the ordered list to
hand the loader, and filtering made it the subset that existed at the moment the
policy was built — so a workspace with no rules directory *yet* reported one path
instead of two.

The tolerance belongs where "this path has no rules" is actually decided.
`loadRulesFromDir` now treats `ENOTDIR` the same as a missing directory, so every
caller shares one unfiltered search order and none of them filters. Recorded as
A6 in the design.

## Public API

- `internal/workspace`: `Policy`, `Mode`, `Resolve`, `ResolveForCWD`, and the
  methods `InWorkspace`, `CatalogScope`, `RulePathsForModel`, `RuleWritePath`,
  `PopulatorPathsFor`. `Context` and `NewContext` removed.
- `internal/datalog`: `loadRulesFromDir` tolerates a non-directory path.
- `pkg/factstore`: no signature change. `Workspace.RulePaths` is unchanged in
  meaning and now comes from the policy.
- `cmd`: `wsCtx` → `wsPolicy`; `ruleSearchPaths` deleted; `queryRulePaths` and
  `rulePathsForModel` added as the two fallback-bearing helpers for callers
  invoked without the Cobra lifecycle.

## Tests

`internal/workspace/policy_test.go` covers the contract cases the
recommendation asks for: global-only, local-only (repo precedes global for
schema/definitions/models), shadowed (asserted *through the loader*, so it tests
the precedence rather than the slice order), nested workspaces (innermost wins
outright), `RulePathsForModel` inside and outside a workspace, `RuleWritePath`,
`PopulatorPathsFor`, `CatalogScope`, and that the search order is not filtered to
what exists.

`pkg/factstore/resolve_test.go` asserts CLI/library parity directly:
`DiscoverWorkspace` returns the policy's rule paths verbatim.

`internal/datalog/loader_rule_paths_test.go` pins the loader tolerance: a missing
path and a path that is a file both contribute nothing, without erroring.

## Not done here

`Policy` is deliberately not threaded into `internal/` constructors that take
explicit path lists today (the schema inferrer, the importer). A path list is
the right dependency for them; the policy is what *produces* the list. That
separation is what lets Recommendation 6 cache compiled schema state keyed on
those paths.

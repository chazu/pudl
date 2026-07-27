# Design: one workspace policy, shared by CLI and library

**Date:** 2026-07-27
**Implements:** Recommendation 5 from `docs/architecture-improvement-report.md`
**Status:** stabilized after adversarial review (§6); ready to execute

## 1. What is scattered today

Workspace precedence is implemented, but it is *re-implemented* per call site.

| Decision | Where it is made |
|---|---|
| schema search paths | `workspace.Context.SchemaSearchPaths`, read via `cmd.effectiveSchemaPaths` |
| definition search paths | `workspace.Context.DefinitionSearchPaths` (no reader) |
| rule paths, for `pudl query` | `cmd/query_helpers.go:19-21`, assembled inline |
| rule paths, for model checks | `cmd/run_checks.go:40-56`, assembled inline, different order |
| rule paths, for the public API | `pkg/factstore/resolve.go:44-49`, assembled inline again |
| rule write target | `cmd/rule.go:77-80`, assembled inline |
| model search dirs | `cmd/model_list.go:31-38`, which calls `workspace.Discover(".")` **a second time** |
| effective origin | `workspace.Context.EffectiveOrigin`, read in three places |

Four independent assemblies of the same "global then repo" rule, and a second
workspace discovery that could in principle disagree with the one the CLI
resolved at startup. The report's success measure — "workspace resolution is
identical across CLI and library APIs" — holds today only by coincidence, not by
construction.

## 2. The policy value

One value, resolved once per invocation, carrying every path decision.

```go
type Policy struct {
    Workspace       *Workspace // nil in global-only mode
    GlobalDir       string
    Mode            Mode       // ModeWorkspace | ModeGlobal
    EffectiveOrigin string

    SchemaSearchPaths     []string
    DefinitionSearchPaths []string
    RuleSearchPaths       []string
    ModelSearchPaths      []string
    PopulatorSearchPaths  []string
}

func Resolve(cwd, globalDir string) (*Policy, error)
func (p *Policy) InWorkspace() bool
func (p *Policy) RulePathsForModel(modelDir string) []string
func (p *Policy) RuleWritePath() (string, error)
func (p *Policy) CatalogScope() string
```

`Context` is replaced rather than wrapped: it has two fields nobody reads and
one nobody can extend without touching every call site.

### 2.1 Orderings are preserved exactly, including one that looks wrong

- **Schema / definitions / models:** repo first, then global. Repo wins; these
  lists are searched front-to-back.
- **Rules:** global first, then repo. `LoadRulesFromPaths` walks its arguments in
  *reverse* and skips already-seen rule names, so **later paths win**. Repo rules
  therefore shadow global ones, which is the same precedence expressed the other
  way round.
- **Model rules** (`RulePathsForModel`): `[global, modelDir, repo]`, so a repo
  workspace's rules shadow the model's own.

That last one is arguably backwards — a model's `rules/` directory is more
specific than the repo it sits in — but it is what `cmd/run_checks.go` does
today, and changing rule precedence is a behaviour change nobody asked for.
Preserved verbatim and flagged here rather than quietly "fixed".

### 2.2 `CatalogScope` is the effective origin, and that is all it can be

Recommendation 5 lists "catalog scope" among the policy's fields. The only
catalog scope a *workspace* implies is the origin its imports and runs are
recorded under — which is `EffectiveOrigin`. A run's `--catalog-scope` is not
workspace policy: it names a specific snapshot or origin per invocation, and
Defect 3 made it explicitly **required** rather than inferred, precisely because
inferring it let a replay compare against every observation in the catalog.
`CatalogScope()` is therefore a named accessor over `EffectiveOrigin`, and the
report's field is answered without reintroducing an inference Defect 3 removed.

### 2.3 The non-directory tolerance belongs in the loader, not the path list

`ruleSearchPaths` filtered its output to existing directories; `loadQueryRules`
and `factstore` did not. The filter is almost cosmetic — `loadRulesFromDir`
already returns nil for a missing directory — but not entirely: a path that
exists and is a *file* makes `os.ReadDir` fail with ENOTDIR, which the filter
caught and the unfiltered callers surfaced as a load error.

*Revised during implementation (see A6).* Filtering in the policy was the wrong
place: `RuleSearchPaths` is a search **order**, and a caller reporting where
rules are looked for needs the whole list, not the subset that exists at this
instant. `loadRulesFromDir` now treats ENOTDIR the same as a missing directory —
both are "no rules here" from the caller's point of view — so every caller
shares one unfiltered order and none of them filters.

## 3. Blast radius

| Surface | Change |
|---|---|
| `internal/workspace` | `Policy`, `Resolve`; `Context` removed |
| `cmd` | `wsCtx` becomes `wsPolicy`; four inline assemblies deleted; `modelSearchDirs` stops re-discovering |
| `pkg/factstore` | `DiscoverWorkspace` delegates to `Resolve`; `Workspace` struct unchanged |
| behaviour | none intended, beyond the ENOTDIR forgiveness in §2.3 |

## 4. Tests (the contract cases Recommendation 5 asks for)

| Case | Assertion |
|---|---|
| Global-only | no workspace; every list is the global one; origin `global` |
| Local-only (repo with `.pudl`) | repo paths precede global for schema/definitions/models |
| Shadowed | a rule name in both resolves to the repo's |
| Nested workspaces | the innermost `.pudl/workspace.cue` wins outright |
| CLI vs library | `factstore.DiscoverWorkspace` returns the policy's rule paths verbatim |
| `RulePathsForModel` | `[global, modelDir, repo]`, order preserved |
| `RuleWritePath` | repo in a workspace; error outside one |
| Non-directory rule path | skipped, not an error |
| `CatalogScope` | equals `EffectiveOrigin` |

## 5. Non-goals

- No change to *what* any path resolves to. This is a consolidation; the one
  documented behaviour difference is §2.3.
- No new workspace configuration keys. `workspace.cue` is untouched.
- `Policy` is not threaded into `internal/` constructors that take explicit
  paths today (the inferrer, the importer). They take path lists, which is the
  right dependency for them; the policy is what *produces* those lists.

## 6. Adversarial review

**A1 — "Replacing `Context` breaks anything holding one."** *Checked: nothing
outside `cmd` does.* `workspace.Context` is referenced only by `cmd/root.go` and
`cmd/workspace_paths.go`, plus its own tests. `pkg/` never exposed it.

**A2 — "`RulePathsForModel` preserving a wrong ordering bakes in a bug."**
*Accepted deliberately, and it is the smaller risk.* Changing which rules shadow
which changes what a model's checks evaluate — silently, for anyone who has both
a repo rule and a model rule of the same name. That is a change to run semantics
disguised as a refactor. Recorded in §2.1 so a later decision can take it on
its own merits.

**A3 — "`CatalogScope()` as an alias for `EffectiveOrigin` is a fake field."**
*Half-accepted.* It is an alias, and §2.2 says so. What it buys is a name for the
question call sites actually ask, and a documented place to say why a run's
`--catalog-scope` is not this. The alternative — a `CatalogScope` field the
policy cannot fill — would be worse.

**A4 — "`modelSearchDirs` re-discovering the workspace is harmless; it always
agrees."** *Rejected.* It agrees only while nothing changes the working
directory between `PersistentPreRun` and the call, and while `Discover(".")`
sees the same tree as `Discover(cwd)`. Two answers to one question is the defect
class this recommendation exists to remove, whether or not they currently
differ.

**A6 — "Filtering in the policy loses information the policy exists to carry."**
*Landed, after a test caught it.* `factstore.DiscoverWorkspace` documents
`RulePaths` as the ordered list to hand the loader; filtering made it the subset
that existed when the policy was built, so a workspace with no rules directory
yet reported one path instead of two. The tolerance moved into
`loadRulesFromDir`, which is where "this path has no rules" is actually decided.

**A5 — "Filtering rule paths hides a typo: a misspelled rules directory silently
contributes nothing."** *True, and true today too* — three of the four call
sites already tolerate a missing directory, because a workspace legitimately may
not have one. The change makes the fourth consistent rather than introducing the
tolerance.

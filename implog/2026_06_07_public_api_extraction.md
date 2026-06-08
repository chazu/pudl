# Public API extraction: fact store + datalog (Phase 1)

Date: 2026-06-07

## Goal

Let external Go applications query pudl data stores (global `~/.pudl` and
repo-scoped `.pudl/`) through `pkg/factstore` and `pkg/eval` without importing any
`pudl/internal/*` package. The Go `internal/` rule already blocks external import of
internal packages; the work was making the `pkg/` facade complete and non-leaky.

## Problems addressed

1. **Leaky facade** — `factstore.DB()` returned `*database.CatalogDB` (an internal
   type the caller cannot name); `pkg/eval`'s `New*EDB`/`NewEvaluator` returned
   internal `datalog.*` types.
2. **Incomplete facade** — the live query path (partition → SQL → recursive) lived
   inline in `cmd/query.go`. `pkg/eval` only exposed the legacy in-memory evaluator,
   so external tools could not actually run a query.
3. **Dead code** — the in-memory evaluator and its EDB sources were no longer in the
   query path.
4. **Latent recursion bug** (found via new test) — the inline routing ran the SQL
   evaluator on base rules first and only fell back to `EvalRecursive` when SQL
   returned nothing. For a transitive-closure relation with a base rule, SQL
   returned the base tuples (non-empty), so the recursive expansion was never run —
   `pudl query` returned only direct edges. Fixed in the extracted orchestrator.

## Changes

### internal/datalog
- **New `match.go`** — moved shared helpers `matchConstraints`, `valuesEqual`,
  `toFloat64` out of the deleted `eval.go` (still used by `sql_eval.go`).
- **New `query.go`** — `Evaluate(db, rules, relation, constraints, scope) ([]Tuple, error)`:
  single orchestrator shared by the CLI and the public API. Routing fix: if the
  queried relation is the head of a recursive rule, `EvalRecursive` is authoritative
  (it seeds base rules and computes the full closure); otherwise the SQL evaluator
  runs, with an empty-result fallback to `EvalRecursive` for relations that
  transitively depend on a recursive one.
- **Deleted** `eval.go`, `eval_test.go`, `edb.go`, `index.go` (legacy in-memory
  evaluator, `EDB`/`MultiEDB`/`FactsEDB`/`CatalogEDB`/`MemoryEDB`, and its index).
- **Trimmed `types.go`** — removed `Binding` type and its `Apply` method (only the
  legacy evaluator used them). `ParseTerm` kept (loader uses it).

### cmd/query.go
- Replaced the inline partition/SQL/recursive block with a single
  `datalog.Evaluate(...)` call. Behavior is identical except the recursion bug fix.

### pkg/eval (rewritten — rules + types only)
- Types: `Rule`, `Atom`, `Term`, `Tuple` (aliases). Funcs: `Var`, `Val`,
  `LoadRulesFromPaths`, `ParseRulesFromSource`.
- Removed `EDB`, `NewEvaluator`, `NewMultiEDB`, `NewFactsEDB`, `NewCatalogEDB`.

### pkg/factstore (rewritten + new resolve.go)
Public API:
- `Open(pudlDir string) (*Store, error)`, `(*Store) Close() error`
- `(*Store) AddFact(Fact) (Fact, error)`
- `(*Store) QueryFacts(FactFilter) ([]Fact, error)`
- `(*Store) RetractFact(id string) error`
- `(*Store) InvalidateFact(id string) error`
- `(*Store) Query(QueryOptions) ([]Tuple, error)` — Datalog query; replaces the
  removed `DB()` accessor.
- Types: `Fact`, `FactFilter`, `Rule`, `Tuple` (aliases; `Rule`/`Tuple` re-exported
  so query-only consumers need only this package).
- `QueryOptions{Relation, Constraints, Rules, ValidAt, TxAt}`.
- `GlobalDir() string`
- `DiscoverWorkspace(cwd string) (*Workspace, error)` →
  `Workspace{RepoDir, GlobalDir, RulePaths}`. `RulePaths` is global-first then repo,
  matching `pudl query` (loader gives later paths priority, so repo rules shadow
  global).

No exported signature references a `pudl/internal/*` type.

## Tests

- `pkg/factstore/factstore_test.go` — base relation, constrained, derived
  non-recursive (SQL path), derived recursive (fixpoint, transitive closure = 6),
  fact round-trip.
- `pkg/eval/eval_test.go` — `ParseRulesFromSource`, `Var`/`Val`.
- `pkg/factstore/resolve_test.go` — `GlobalDir`, workspace discovery (global-only and
  repo modes, rule-path ordering).
- `CGO_ENABLED=0 go test ./...` green (26 packages), `go vet` clean.

## Deferred

Phase 2 (catalog-as-datalog bridge): expose `catalog_entries` as a `catalog_entry`
datalog relation via a SQL view + the compiler's `CompileOptions.TableOverrides`.
Not started; see docs/plan.md.

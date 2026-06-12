# Library API (`pkg/factstore`, `pkg/eval`)

PUDL exposes a small public Go API so external programs can read and query a
PUDL data store — global (`~/.pudl`) or repo-scoped (`.pudl/`) — without depending
on PUDL's internal packages. Everything under `internal/` is import-restricted by
the Go compiler; only `pkg/factstore` and `pkg/eval` are importable, and neither
exposes an `internal/` type in its API (plain-data types are re-exported as
aliases).

The module path is `pudl`, so imports are `pudl/pkg/factstore` and `pudl/pkg/eval`.

## `pkg/factstore`

`Store` is the single handle for a data store: fact CRUD, Datalog queries, and
catalog listing.

```go
func Open(pudlDir string) (*Store, error)
func (s *Store) Close() error

// Bitemporal fact store
func (s *Store) AddFact(f Fact) (Fact, error)
func (s *Store) QueryFacts(filter FactFilter) ([]Fact, error)
func (s *Store) RetractFact(id string) error
func (s *Store) InvalidateFact(id string) error
func (s *Store) FactHistory(relation string) ([]Fact, error)

// Atomic check-and-write
func (s *Store) Transact(fn func(tx *Tx) error) error

// Datalog query
func (s *Store) Query(opts QueryOptions) ([]Tuple, error)

// Catalog listing
func (s *Store) ListCatalog(filter CatalogFilter, query CatalogQuery) (*CatalogResult, error)
```

Re-exported types: `Fact`, `FactFilter`, `Rule`, `Tuple`, `Tx`, `CatalogEntry`,
`CatalogFilter`, `CatalogQuery`, `CatalogResult`.

### `Transact`

`Transact` runs its callback inside a single store transaction that holds the
write lock from the start: every read the callback performs and every write it
lands form one atomic, serialized unit. Use it for check-then-write sequences —
read the current facts, validate an invariant, then append — that must not
interleave with concurrent writers (the classic TOCTOU race between a legality
check and its write). The `Tx` handle offers `AddFact`, `RetractFact`,
`InvalidateFact`, `QueryFacts`, and `FactHistory` with the same semantics as
the `Store` methods. Returning an error rolls back every write made through
the `Tx`; concurrent transactions block until the holder finishes, bounded by
the store's busy timeout.

```go
err := st.Transact(func(tx *factstore.Tx) error {
    facts, err := tx.QueryFacts(factstore.FactFilter{Relation: "dlktk/preference"})
    if err != nil {
        return err
    }
    if wouldCreateCycle(facts, winner, loser) {
        return fmt.Errorf("preference would create a cycle")
    }
    _, err = tx.AddFact(factstore.Fact{Relation: "dlktk/preference", Args: args})
    return err
})
```

### `QueryOptions`

```go
type QueryOptions struct {
    Relation    string                 // head relation to query (required)
    Constraints map[string]interface{} // filter results by arg value
    Rules       []Rule                 // rules to evaluate (load with pkg/eval)
    ValidAt     *int64                 // bitemporal: facts valid at this Unix time
    TxAt        *int64                 // bitemporal: facts known at this Unix time
}
```

Both `ValidAt` and `TxAt` nil evaluates over current facts; setting either evaluates
over the historical `facts` table. A query against a base relation with no producing
rule returns matching facts directly.

### Store/workspace resolution

```go
func GlobalDir() string                              // ~/.pudl
func DiscoverWorkspace(cwd string) (*Workspace, error)

type Workspace struct {
    RepoDir   string   // repo-scoped .pudl dir, or "" outside a workspace
    GlobalDir string   // ~/.pudl
    RulePaths []string // rule dirs, global first then repo (repo shadows global)
}
```

`DiscoverWorkspace` walks up from `cwd` for a repo workspace and assembles the rule
search paths exactly as `pudl query` does. Pass `RulePaths` to
`eval.LoadRulesFromPaths`.

## `pkg/eval`

Rule loading, parsing, and rule types.

```go
func LoadRulesFromPaths(paths ...string) ([]Rule, error) // load *.cue rules from dirs
func ParseRulesFromSource(source string) ([]Rule, error) // parse rules from a CUE string
func Var(name string) Term                               // variable term
func Val(v interface{}) Term                             // ground value term
```

Types: `Rule`, `Atom`, `Term`, `Tuple` (same underlying types as `factstore`'s
`Rule`/`Tuple`).

## Example

```go
package main

import (
    "fmt"
    "os"

    "pudl/pkg/eval"
    "pudl/pkg/factstore"
)

func main() {
    cwd, _ := os.Getwd()

    // Resolve rule search paths (repo + global) and load rules.
    ws, _ := factstore.DiscoverWorkspace(cwd)
    rules, _ := eval.LoadRulesFromPaths(ws.RulePaths...)

    // Open the global store and run a Datalog query.
    st, _ := factstore.Open(factstore.GlobalDir())
    defer st.Close()

    out, _ := st.Query(factstore.QueryOptions{Relation: "at_risk", Rules: rules})
    for _, t := range out {
        fmt.Printf("%s %v\n", t.Relation, t.Args)
    }

    // List catalog entries directly (typed, paginated).
    res, _ := st.ListCatalog(factstore.CatalogFilter{Origin: "prod"}, factstore.CatalogQuery{})
    fmt.Printf("%d catalog entries\n", len(res.Entries))
}
```

## Querying the catalog from Datalog

The catalog is exposed to Datalog as the built-in `catalog_entry` relation (see
[datalog.md](datalog.md#catalog-catalog_entry)). It is **join-only**: reference it in
a rule body to join facts against catalog data. Querying it directly
(`QueryOptions{Relation: "catalog_entry"}` with no producing rule) returns an error —
use `ListCatalog` for direct catalog access. The name is reserved, so `AddFact`
rejects facts asserted under `catalog_entry`.

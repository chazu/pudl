# Library API (`pkg/factstore`, `pkg/eval`)

PUDL exposes a small public Go API so external programs can read and query a
PUDL data store — global (`~/.pudl`) or repo-scoped (`.pudl/`) — without depending
on PUDL's internal packages. Everything under `internal/` is import-restricted by
the Go compiler; only `pkg/factstore` and `pkg/eval` are importable, and neither
exposes an `internal/` type in its API (plain-data types are re-exported as
aliases).

The module path is `github.com/chazu/pudl`, so imports are
`github.com/chazu/pudl/pkg/factstore` and `github.com/chazu/pudl/pkg/eval`.

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
func (s *Store) LatestFactVersion(id string) (*Fact, error)
func (s *Store) FactVersions(id string) ([]Fact, error)

// Atomic check-and-write
func (s *Store) Transact(fn func(tx *Tx) error) error

// Datalog query
func (s *Store) Query(opts QueryOptions) ([]Tuple, error)

// Catalog listing
func (s *Store) ListCatalog(filter CatalogFilter, query CatalogQuery) (*CatalogResult, error)
```

Re-exported types: `Fact`, `FactFilter`, `Rule`, `Tuple`, `Tx`, `CatalogEntry`,
`CatalogFilter`, `CatalogQuery`, `CatalogResult`.

`AddFact` and `Tx.AddFact` return the stored fact on an identical-ID replay,
including original provenance and terminal temporal bounds. They reject reuse
of an ID for different identity content. Fact ID canonicalization preserves
exact decimal numbers and normalizes equivalent numeric spellings. See
[existing-store compatibility](facts.md#existing-store-compatibility) for the
one-time projection repair and handling of IDs created by older releases.

Transaction time is append-only. `InvalidateFact` ends the belief in the open
version and records a successor version with `valid_end` set; its
`Fact.Supersedes` names the old ID, and the old row is never edited, so
`TxAt` queries for earlier moments return what was believed then.
`LatestFactVersion` follows an ID to the newest version, and `FactVersions`
returns the whole chain. `RetractFact` and `InvalidateFact` act on the newest
version of the ID's fact. See [retraction vs
invalidation](facts.md#retraction-vs-invalidation).

`Fact.TxSeq`/`Fact.TxEndSeq` record the store-wide write sequence that
created and ended each version. `FactFilter.TxSeqAt` queries the state right
after a given write. That is exact where whole-second `TxAt` cannot separate
two writes in the same second (see [whole seconds and write
sequence](facts.md#whole-seconds-and-write-sequence)). `QueryFacts` and
`FactHistory` return an empty, non-nil slice when nothing matches.

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

Every supplied constraint is applied as a conjunction. Recursive evaluation
supports multiple derived atoms in one rule body, including repeated occurrences
of the same relation. It combines new tuples with accumulated results until no
new tuples remain. The existing 100-iteration limit and rejection of recursive
aggregation still apply.

### Numeric query contract

`Store.Query` returns integral fact values and SQLite INTEGER results as Go
`int64`, including `count` results. Non-integral fact values and SQLite REAL
results use `float64`. This changes the previous behavior that returned every
number as `float64`; callers asserting that type must also handle `int64`.
JSON output preserves the integer digits. Nested base-fact arguments follow
the same conversion rules.

The supported input domain is signed 64-bit integers
(`-9223372036854775808` through `9223372036854775807`) and non-integral decimals
whose value survives `float64` conversion and Go JSON serialization unchanged.
Equivalent spellings such as `9007199254740993` and `9.007199254740993e15`
compare equally without converting the integer through floating point.
`0.1` is supported; `0.123456789012345678901` is rejected. Integer values
outside int64, precision-losing decimals, overflow, and underflow produce an
`unsupported query number` error when accessed, rather than rounded results.

Constraints accept Go numeric types and `json.Number`; use `int64` or
`json.Number` for exact integer operands. A supplied `float64` already denotes
a binary value, so PUDL cannot recover digits a caller previously rounded.
String constraints remain strings. CUE rule loaders retain numeric
`Term.Value` operands as `json.Number` until compilation validates them.
This contract applies to current and historical queries, SQL joins and filters,
and recursive temporary tables.

Raw `Fact.Args` returned by `QueryFacts` and `FactHistory`, stored IDs, and
`AddFact` remain unchanged and retain numbers outside the query domain.
Arithmetic over REAL values, including mixed/real `sum` and computed decay
scores, retains SQLite's binary64 semantics; this is not an arbitrary-precision
decimal arithmetic engine. Integer-only `sum` retains SQLite's integer overflow
error. Built-in catalog columns describe their native SQLite values.

### Store/workspace resolution

```go
func GlobalDir() string                              // ~/.pudl
func DiscoverWorkspace(cwd string) (*Workspace, error)

type Workspace struct {
    RepoDir   string   // repo-scoped .pudl dir, or "" outside a workspace
    GlobalDir string   // ~/.pudl
    RulePaths []string // rule dirs, explicit dependencies then repo (repo wins)
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

    "github.com/chazu/pudl/pkg/eval"
    "github.com/chazu/pudl/pkg/factstore"
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

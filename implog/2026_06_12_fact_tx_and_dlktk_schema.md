# Transactional check-and-write + pudl/dlktk built-in schema

Date: 2026-06-12

## Goal

Two pudl-side features requested by dlktk's project analysis (dlktk
`ANALYSIS.md`):

1. **Close the TOCTOU race (§1.4).** Every dlktk move is read-graph → legality
   check → write as separate `pkg/factstore` calls, so two concurrent agents
   (e.g. `prefer A B` and `prefer B A` in separate processes) can both pass the
   cycle check and both land, corrupting the store. The fix is check-and-write
   inside one transaction, which needed a pudl API addition.
2. **Register the `pudl/dlktk` CUE schema package (dlktk design §3.7).** Ship
   the typed args shape of every `dlktk/*` relation as a built-in bootstrap
   schema package, so dlktk facts are interpretable by pudl's CUE tooling and
   other fact-store consumers rather than being opaque JSON blobs.

## Public API implemented

### `pkg/factstore`

```go
// Tx is a fact-store transaction handle, passed to the Transact callback.
type Tx = database.FactTx

// Transact runs fn inside a single store transaction that holds the write
// lock from the start. An error from fn rolls back every write.
func (s *Store) Transact(fn func(tx *Tx) error) error
```

`Tx` methods (same semantics as the `Store` equivalents, all inside the one
transaction, reads see uncommitted writes):

```go
func (t *FactTx) AddFact(f Fact) (Fact, error)
func (t *FactTx) RetractFact(id string) error
func (t *FactTx) InvalidateFact(id string) error
func (t *FactTx) QueryFacts(filter FactFilter) ([]Fact, error)
func (t *FactTx) FactHistory(relation string) ([]Fact, error)
```

### `internal/database`

```go
func (c *CatalogDB) WithFactTx(fn func(*FactTx) error) error
```

## Mechanism

`WithFactTx` pins a dedicated `*sql.Conn` and issues `BEGIN IMMEDIATE`
explicitly: SQLite takes the database write lock at BEGIN rather than at the
first write statement, so the whole read–check–write span is serialized
against every other writer — in this process or another (multi-process CLI
invocations included, which is dlktk's open case; its MCP mutex only covered
one process). Concurrent transactions block on the existing
`busy_timeout(5000)` pragma. Commit/rollback are explicit `COMMIT`/`ROLLBACK`
on the same connection; a deferred rollback covers error and panic paths.

The core fact operations were extracted from the `CatalogDB` methods into
package-level functions over a small `dbtx` interface (`Exec`/`Query`/
`QueryRow`, satisfied by `*sql.DB`, `*sql.Tx`, and a `*sql.Conn` adapter):
`addFactIn`, `retractFactIn`, `invalidateFactIn`, `queryFactsIn`,
`factHistoryIn`. The `CatalogDB` methods keep their exact prior behavior
(per-call transactions for writes); `FactTx` calls the same functions on the
open connection, so `current_facts` sync, content-addressed dedup, and
reserved-relation rejection are identical on both paths. The three duplicated
nine-column row-scan loops collapsed into one `scanFactRows` helper.

## Built-in `pudl/dlktk` schema

`internal/importer/bootstrap/pudl/dlktk/dlktk.cue` (package `dlktk`) defines
`#Discussion`, `#Node`, `#Link`, `#IssueCard`, `#Preference`, `#Decision`, and
the `#byRelation` relation→schema binding, mirroring the `pudl/dlktk` package
dlktk ships from `internal/discover`. `BootstrapPackages()` discovers it
automatically (embed-FS walk), so `pudl schema list` marks it built-in and the
doctor's reserved-namespace check accepts it. Added to the
`ensureBasicSchemas` check list so existing initialized stores pick it up on
next import, and stubbed in `test/testutil.AddBootstrapSchemas` (which stubs
every checked path to keep test workspaces from triggering a full bootstrap
copy).

## Files

- `internal/database/facts.go` — ops extracted over `dbtx`; shared `scanFactRows`
- `internal/database/facts_tx.go` — new: `dbtx`, conn adapter, `FactTx`, `WithFactTx`
- `internal/database/current_facts.go` — `insertCurrentFact`/`deleteCurrentFact` take `dbtx`
- `internal/database/facts_tx_test.go` — new: commit, rollback, read-your-writes,
  retract/invalidate, and a concurrent check-and-write serialization test
- `pkg/factstore/factstore.go` — `Tx` alias + `Transact`
- `pkg/factstore/factstore_test.go` — Transact commit/rollback test
- `internal/importer/bootstrap/pudl/dlktk/dlktk.cue` — new built-in package
- `internal/importer/cue_schemas.go` — dlktk in `ensureBasicSchemas` checks
- `test/testutil/temp_dirs.go` — dlktk stub in `AddBootstrapSchemas`
- `docs/library-api.md`, `docs/facts.md`, `docs/plan.md` — documented

## Testing

`CGO_ENABLED=0 go test` green on all packages buildable in this environment
(`internal/pithdriver` and `cmd` need the `../pith` sibling checkout; the
pre-existing `TestNewCatalogDB/nonexistent_path` failure is
environment-dependent — running as root makes `/nonexistent` creatable — and
fails identically on a clean tree). The serialization test runs two concurrent
check-then-write transactions against the same invariant and asserts exactly
one write lands.

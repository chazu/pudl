package cmd

import (
	"fmt"

	"github.com/chazu/pudl/internal/database"
)

// runCatalog is the one catalog handle a `pudl run` owns for its whole lifetime.
// Every phase that touches the catalog borrows it instead of opening its own.
//
// The run path used to call database.NewCatalogDB eleven times — the inventory
// branch, the status write, the run record's start and finish, promotion,
// checks, the dependency reconcile, the upstream-freshness read, the observe
// ingest, the manifest ingest and the drift observation. Each open re-runs
// createTables and every migration (idempotent by design), so a single run did
// eleven schema setups and left eleven independent lifecycles to get right.
//
// Opening is lazy and memoized: the handle is created on first borrow, so a run
// that never reaches a catalog-touching phase never opens one. That is what
// keeps `--dry-run` honest — it promises no catalog writes, and under a lazy
// handle it does not so much as create the database file.
//
// The two accessors state what used to be inherited by accident from wherever
// the open happened to sit. Opening in-place let each phase's failure semantics
// come from its call site: the inventory, checks and populate paths treat "the
// catalog cannot be opened" as fatal, while the status, audit, dependency and
// promotion paths treat it as a warning and must never fail the run. Sharing one
// handle erases that context, so a borrow now has to name which it is —
// required() or optional().
type runCatalog struct {
	dir string

	// opened records that the open has been attempted, so a failure is not
	// retried once per phase (and a nil db is not mistaken for "not yet tried").
	opened bool
	db     *database.CatalogDB
	err    error
}

// newRunCatalog prepares the run's handle over the given pudl dir. Nothing is
// opened until the first borrow.
func newRunCatalog(pudlDir string) *runCatalog { return &runCatalog{dir: pudlDir} }

// Dir is the pudl dir this catalog lives under. Phases that write raw files
// beside their catalog rows (the manifest ingest) take the path from here rather
// than re-reading the config, so the rows and the files they point at cannot end
// up under different roots.
func (c *runCatalog) Dir() string { return c.dir }

// open resolves the handle once and remembers the outcome, success or failure.
func (c *runCatalog) open() (*database.CatalogDB, error) {
	if !c.opened {
		c.opened = true
		c.db, c.err = database.NewCatalogDB(c.dir)
	}
	return c.db, c.err
}

// required borrows the handle for a phase that cannot do its job without the
// catalog — the inventory drift, the checks, the observe ingest. The error is
// the caller's to propagate and it fails the run.
func (c *runCatalog) required() (*database.CatalogDB, error) {
	db, err := c.open()
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	return db, nil
}

// optional borrows the handle for a best-effort phase — the status write, the
// run's audit row, the dependency reconcile, promotion, the drift observation.
// It returns a nil handle when the catalog could not be opened, and the caller
// must return rather than fail the run.
//
// The open error comes back with it so the caller can report the write it is
// dropping. Best-effort is not the same as silent: a status write that vanishes
// leaves the previous run's verdict standing, which is exactly how a stale
// `clean` used to survive. The two callers that deliberately stay quiet
// (finishRunRecord, whose start already warned, and the read-only upstream
// check) say so where they ignore it.
func (c *runCatalog) optional() (*database.CatalogDB, error) {
	return c.open()
}

// Close releases the handle. It is a no-op when no phase ever borrowed one.
func (c *runCatalog) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

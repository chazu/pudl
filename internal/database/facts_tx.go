package database

import (
	"context"
	"database/sql"

	"github.com/chazu/pudl/internal/errors"
)

// dbtx abstracts the executor a fact operation runs against: *sql.DB for
// standalone reads, *sql.Tx for the per-call write transactions, and connExec
// for operations inside an open FactTx.
type dbtx interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// connExec adapts *sql.Conn (which only exposes context-taking methods) to
// the dbtx interface.
type connExec struct {
	ctx  context.Context
	conn *sql.Conn
}

func (c connExec) Exec(query string, args ...interface{}) (sql.Result, error) {
	return c.conn.ExecContext(c.ctx, query, args...)
}

func (c connExec) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return c.conn.QueryContext(c.ctx, query, args...)
}

func (c connExec) QueryRow(query string, args ...interface{}) *sql.Row {
	return c.conn.QueryRowContext(c.ctx, query, args...)
}

// FactTx is a fact-store transaction: every read and write performed through
// it executes inside one SQLite transaction that holds the database write
// lock from the moment it begins (BEGIN IMMEDIATE). A check-then-write
// sequence — read facts, validate an invariant, append — therefore cannot
// interleave with any other writer, in this process or another. Obtain one
// with CatalogDB.WithFactTx; never retain it past the callback.
type FactTx struct {
	q connExec
}

// AddFact inserts a fact inside the transaction. Same semantics as
// CatalogDB.AddFact: content-addressed ID, idempotent re-add, current_facts
// kept in sync.
func (t *FactTx) AddFact(f Fact) (Fact, error) {
	return addFactIn(t.q, f)
}

// RetractFact marks a fact as retracted (sets tx_end) inside the transaction.
func (t *FactTx) RetractFact(id string) error {
	return retractFactIn(t.q, id)
}

// InvalidateFact marks a fact as no longer valid (sets valid_end) inside the
// transaction.
func (t *FactTx) InvalidateFact(id string) error {
	return invalidateFactIn(t.q, id)
}

// QueryFacts returns facts matching the filter with bitemporal scoping,
// reading the transaction's view (uncommitted writes included).
func (t *FactTx) QueryFacts(filter FactFilter) ([]Fact, error) {
	return queryFactsIn(t.q, filter)
}

// FactHistory returns every fact ever recorded for a relation, reading the
// transaction's view.
func (t *FactTx) FactHistory(relation string) ([]Fact, error) {
	return factHistoryIn(t.q, relation)
}

// WithFactTx runs fn inside a single immediate-mode SQLite transaction. The
// write lock is taken up front, so the whole read–check–write span is
// serialized against every other writer (concurrent callers block, bounded by
// the connection's busy_timeout). If fn returns an error the transaction is
// rolled back and the error returned; otherwise it is committed.
func (c *CatalogDB) WithFactTx(fn func(*FactTx) error) error {
	ctx := context.Background()
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to acquire connection", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to begin immediate transaction", err)
	}

	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(ctx, "ROLLBACK")
		}
	}()

	if err := fn(&FactTx{q: connExec{ctx: ctx, conn: conn}}); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to commit transaction", err)
	}
	committed = true
	return nil
}

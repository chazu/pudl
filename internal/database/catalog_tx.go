package database

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/errors"
)

// CatalogWriter is the set of catalog operations one recorded step performs. It
// is satisfied by *CatalogDB (each call its own implicit transaction) and by
// *CatalogTx (all calls inside one).
//
// It exists so the ingest paths can be written once and run either way: callers
// that already hold a transaction pass it straight through, and callers that do
// not keep working unchanged.
type CatalogWriter interface {
	AddEntry(entry CatalogEntry) error
	AddCollectionMembership(collectionID, itemID string, itemIndex int) error
	RecordObserveSnapshot(snapshot ObserveSnapshot) error
	UpdateStatus(targetName, status string) error
	GetEntry(id string) (*CatalogEntry, error)
	FindByContentHash(contentHash string) (*CatalogEntry, error)
	GetLatestObserveByContentHash(targetName, contentHash string) (*CatalogEntry, error)
	GetTargetStatuses() ([]TargetStatus, error)
}

// CatalogTx is one atomic catalog step: every read and write performed through
// it executes inside a single SQLite transaction that holds the write lock from
// the moment it begins (BEGIN IMMEDIATE), and reads see the transaction's own
// uncommitted writes.
//
// The unit is deliberately the *step*, not the run. A converge run shells out to
// mu for minutes at a time; a transaction spanning the run would hold the
// catalog locked for its whole duration, against other pudl invocations and
// against the run's own other handles. What wants to be atomic is each step that
// records a result — an observation with its snapshot and memberships, or a
// manifest with its per-action statuses — each of which is short and holds no
// lock across a subprocess.
//
// Obtain one with CatalogDB.WithCatalogTx; never retain it past the callback.
type CatalogTx struct {
	q connExec
}

func (t *CatalogTx) AddEntry(entry CatalogEntry) error {
	return addEntryIn(t.q, entry)
}

func (t *CatalogTx) RecordObserveSnapshot(snapshot ObserveSnapshot) error {
	return recordObserveSnapshotIn(t.q, snapshot)
}

func (t *CatalogTx) AddCollectionMembership(collectionID, itemID string, itemIndex int) error {
	return addCollectionMembershipIn(t.q, collectionID, itemID, itemIndex)
}

func (t *CatalogTx) UpdateStatus(targetName, status string) error {
	return updateStatusIn(t.q, targetName, status)
}

func (t *CatalogTx) GetEntry(id string) (*CatalogEntry, error) {
	return getEntryIn(t.q, id)
}

func (t *CatalogTx) FindByContentHash(contentHash string) (*CatalogEntry, error) {
	return findByContentHashIn(t.q, contentHash)
}

func (t *CatalogTx) GetLatestObserveByContentHash(targetName, contentHash string) (*CatalogEntry, error) {
	return getLatestObserveByContentHashIn(t.q, targetName, contentHash)
}

func (t *CatalogTx) GetTargetStatuses() ([]TargetStatus, error) {
	return getTargetStatusesIn(t.q)
}

// WithCatalogTx runs fn inside a single immediate-mode SQLite transaction. If fn
// returns an error the transaction is rolled back and the error returned;
// otherwise it is committed.
//
// Rollback covers catalog rows only. Raw payload files staged on disk during a
// step are not transactional and survive a rollback; they are content-addressed
// and inert without a row pointing at them, so an abandoned step leaves unused
// bytes rather than a corrupt catalog.
//
// Cost: the write lock is held for the whole step, where it used to be taken and
// released per row. An observe ingest also stages its raw files inside that
// window, so a concurrent writer waits for the batch rather than interleaving
// with it, bounded by the connection's busy_timeout (5s). Small file writes make
// this tens of milliseconds for a normal observation; a step large enough to
// approach the timeout wants its file staging hoisted out of the transaction,
// which is a change to the ingest loop rather than to this boundary.
func (c *CatalogDB) WithCatalogTx(fn func(*CatalogTx) error) error {
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

	if err := fn(&CatalogTx{q: connExec{ctx: ctx, conn: conn}}); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to commit transaction", err)
	}
	committed = true
	return nil
}

// getTargetStatusesIn is the executor-parameterized form of GetTargetStatuses.
func getTargetStatusesIn(q dbtx) ([]TargetStatus, error) {
	rows, err := q.Query(`
		SELECT target, status, updated_at
		FROM catalog_entries
		WHERE target IS NOT NULL AND target != ''
		GROUP BY target
		HAVING import_timestamp = MAX(import_timestamp)
		ORDER BY target`)
	if err != nil {
		return nil, fmt.Errorf("failed to query target statuses: %w", err)
	}
	defer rows.Close()

	var statuses []TargetStatus
	for rows.Next() {
		var ds TargetStatus
		var statusVal *string
		if err := rows.Scan(&ds.Target, &statusVal, &ds.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan target status: %w", err)
		}
		if statusVal != nil {
			ds.Status = *statusVal
		} else {
			ds.Status = "unknown"
		}
		statuses = append(statuses, ds)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating target statuses: %w", err)
	}

	return statuses, nil
}

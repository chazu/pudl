package database

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/errors"
)

// Fact versioning.
//
// The facts table is append-only in transaction time: no write rewrites what
// the store believed at an earlier moment. Retraction closes a row's belief
// (tx_end). Invalidation closes the open row's belief and inserts a successor
// version whose valid time is bounded (valid_end) and whose `supersedes` column
// names the row it replaces. An as-of query for a moment before the
// invalidation therefore still sees the unbounded belief held at that moment.
//
// tx_seq orders every write in the store. Each write operation allocates one
// sequence number; a row records the sequence of the write that created it
// (tx_seq) and of the write that closed its belief (tx_end_seq). Whole-second
// tx_start/tx_end cannot order two writes in the same second; the sequence can,
// and FactFilter.TxSeqAt queries it exactly.

// maxFactVersions bounds a supersession chain walk. A chain only grows by one
// invalidation of an open version, which ends the chain, so real chains are
// one or two links long; the bound guards against a corrupted cycle.
const maxFactVersions = 1024

// ensureFactVersioning adds the supersession and write-sequence columns, the
// store-wide sequence counter, and backfills sequences for existing rows.
//
// Legacy rows are ordered by their recorded times. Two writes in the same
// second are indistinguishable there, so their backfilled order is a
// reconstruction: a row's creation precedes its closing, and creations precede
// closings within one second. Rows that an older pudl invalidated in place
// (valid_end rewritten with no successor) cannot be repaired: the belief they
// held before that rewrite was not recorded.
func (c *CatalogDB) ensureFactVersioning() error {
	columns := []struct{ name, ddl string }{
		{"supersedes", "ALTER TABLE facts ADD COLUMN supersedes TEXT"},
		{"tx_seq", "ALTER TABLE facts ADD COLUMN tx_seq INTEGER"},
		{"tx_end_seq", "ALTER TABLE facts ADD COLUMN tx_end_seq INTEGER"},
	}
	var missing []string
	for _, col := range columns {
		exists, err := c.columnExists("facts", col.name)
		if err != nil {
			return fmt.Errorf("check facts.%s: %w", col.name, err)
		}
		if !exists {
			missing = append(missing, col.ddl)
		}
	}

	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := append(missing,
		`CREATE INDEX IF NOT EXISTS idx_facts_supersedes ON facts(supersedes) WHERE supersedes IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_facts_tx_seq ON facts(relation, tx_seq)`,
		`CREATE TABLE IF NOT EXISTS fact_sequence (
			id   INTEGER PRIMARY KEY CHECK (id = 1),
			next INTEGER NOT NULL
		)`,
	)
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("fact versioning schema: %w", err)
		}
	}

	last, err := backfillFactSequences(tx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO fact_sequence (id, next) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET next = MAX(next, excluded.next)`, last+1); err != nil {
		return fmt.Errorf("seed fact_sequence: %w", err)
	}
	return tx.Commit()
}

// backfillFactSequences assigns tx_seq/tx_end_seq to rows that lack them,
// continuing after any sequence already assigned. It returns the last sequence
// in use. Memory is two integers per unsequenced row.
func backfillFactSequences(tx *sql.Tx) (int64, error) {
	var last int64
	if err := tx.QueryRow(
		`SELECT MAX(COALESCE(MAX(tx_seq), 0), COALESCE(MAX(tx_end_seq), 0)) FROM facts`).Scan(&last); err != nil {
		return 0, fmt.Errorf("read fact sequences: %w", err)
	}

	type event struct {
		at     int64
		closes bool
		rowid  int64
	}
	rows, err := tx.Query(`SELECT rowid, tx_start, tx_end FROM facts WHERE tx_seq IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("read unsequenced facts: %w", err)
	}
	var events []event
	for rows.Next() {
		var rowid, txStart int64
		var txEnd sql.NullInt64
		if err := rows.Scan(&rowid, &txStart, &txEnd); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, event{at: txStart, rowid: rowid})
		if txEnd.Valid {
			events = append(events, event{at: txEnd.Int64, closes: true, rowid: rowid})
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.at != b.at {
			return a.at < b.at
		}
		if a.closes != b.closes {
			return !a.closes
		}
		return a.rowid < b.rowid
	})

	setStart, err := tx.Prepare(`UPDATE facts SET tx_seq = ? WHERE rowid = ?`)
	if err != nil {
		return 0, err
	}
	defer setStart.Close()
	setEnd, err := tx.Prepare(`UPDATE facts SET tx_end_seq = ? WHERE rowid = ?`)
	if err != nil {
		return 0, err
	}
	defer setEnd.Close()

	for _, e := range events {
		last++
		stmt := setStart
		if e.closes {
			stmt = setEnd
		}
		if _, err := stmt.Exec(last, e.rowid); err != nil {
			return 0, fmt.Errorf("backfill fact sequence: %w", err)
		}
	}
	return last, nil
}

// allocFactSeq reserves the next write sequence number. Every fact write calls
// it first: the UPDATE takes the write lock, so a deferred transaction never
// upgrades from a read snapshot (which SQLite can refuse under contention), and
// sequence order matches commit order. Sequences are monotonic, not dense: an
// idempotent replay or a failed write consumes a number.
func allocFactSeq(q dbtx) (int64, error) {
	var seq int64
	if err := q.QueryRow(
		`UPDATE fact_sequence SET next = next + 1 WHERE id = 1 RETURNING next - 1`).Scan(&seq); err != nil {
		return 0, errors.WrapError(errors.ErrCodeDatabaseError, "failed to allocate fact sequence", err)
	}
	return seq, nil
}

// latestFactVersionIDIn follows supersession forward from id to the newest
// version of the fact. An ID with no successor resolves to itself, whether or
// not it exists; callers look the row up afterwards.
func latestFactVersionIDIn(q dbtx, id string) (string, error) {
	current := id
	for i := 0; i < maxFactVersions; i++ {
		var next string
		err := q.QueryRow(`SELECT id FROM facts WHERE supersedes = ?`, current).Scan(&next)
		if err == sql.ErrNoRows {
			return current, nil
		}
		if err != nil {
			return "", errors.WrapError(errors.ErrCodeDatabaseError, "failed to follow fact supersession", err)
		}
		current = next
	}
	return "", errors.WrapError(errors.ErrCodeDatabaseError,
		fmt.Sprintf("fact %s: supersession chain exceeds %d versions", id, maxFactVersions), nil)
}

// LatestFactVersion returns the newest version of the fact that id names. An
// ID of a version superseded by invalidation resolves to its successor; any
// other ID resolves to itself. Use GetFact for the exact stored version.
func (c *CatalogDB) LatestFactVersion(id string) (*Fact, error) {
	latest, err := latestFactVersionIDIn(c.db, id)
	if err != nil {
		return nil, err
	}
	return c.GetFact(latest)
}

// FactVersions returns every version of the fact that id names, oldest first:
// the versions it supersedes, itself, and the versions that supersede it.
func (c *CatalogDB) FactVersions(id string) ([]Fact, error) {
	first, err := c.GetFact(id)
	if err != nil {
		return nil, err
	}

	chain := []Fact{*first}
	for i := 0; chain[0].Supersedes != "" && i < maxFactVersions; i++ {
		prev, err := c.GetFact(chain[0].Supersedes)
		if err != nil {
			return nil, err
		}
		chain = append([]Fact{*prev}, chain...)
	}
	for i := 0; i < maxFactVersions; i++ {
		last := chain[len(chain)-1].ID
		next, err := scanFact(c.db.QueryRow(selectFactSQL+` WHERE supersedes = ?`, last))
		if err == sql.ErrNoRows {
			return chain, nil
		}
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to follow fact supersession", err)
		}
		chain = append(chain, *next)
	}
	return nil, errors.WrapError(errors.ErrCodeDatabaseError,
		fmt.Sprintf("fact %s: supersession chain exceeds %d versions", id, maxFactVersions), nil)
}

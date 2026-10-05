package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/errors"
)

// Fact represents a single fact in the bitemporal fact store.
// Facts are the EDB for the Datalog evaluator and the storage layer
// for agent observations.
type Fact struct {
	ID         string `json:"id"`
	Relation   string `json:"relation"`
	Args       string `json:"args"`        // JSON object with meaningful keys
	ValidStart int64  `json:"valid_start"` // unix timestamp
	ValidEnd   *int64 `json:"valid_end,omitempty"`
	TxStart    int64  `json:"tx_start"`
	TxEnd      *int64 `json:"tx_end,omitempty"`
	Source     string `json:"source,omitempty"`
	Provenance string `json:"provenance,omitempty"` // JSON
	// TxSeq is the store-wide sequence of the write that recorded this
	// version, and TxEndSeq that of the write that closed its belief. They
	// order writes that share a whole-second tx_start/tx_end. Assigned by the
	// store; values supplied to AddFact are ignored.
	TxSeq    int64  `json:"tx_seq"`
	TxEndSeq *int64 `json:"tx_end_seq,omitempty"`
	// Supersedes names the version this one replaced when an invalidation
	// bounded its valid time. Set by the store only.
	Supersedes string `json:"supersedes,omitempty"`
}

// selectFactSQL selects every fact column in the order scanFactFrom reads.
const selectFactSQL = `SELECT id, relation, args, valid_start, valid_end, tx_start, tx_end,
	source, provenance, tx_seq, tx_end_seq, supersedes FROM facts`

// FactFilter specifies criteria for querying facts.
// ValidAt and TxAt control temporal query mode:
//   - both nil:       AsOfNow (current valid, current tx)
//   - ValidAt set:    AsOfValid (what was true at ValidAt, current knowledge)
//   - TxAt set:       AsOfTransaction (what we believed at TxAt)
//   - both set:       AsOf (what we believed at TxAt about what was true at ValidAt)
//
// TxAt is a whole Unix second and means "after every write committed during or
// before that second": belief intervals are half-open, [tx_start, tx_end). A
// belief recorded and closed within one second is therefore in no whole-second
// state; it remains in FactHistory, and TxSeqAt observes it exactly.
//
// TxSeqAt replaces TxAt with a write sequence number: "after write N". Setting
// both is an error.
type FactFilter struct {
	Relation string // required
	ValidAt  *int64 // optional: filter by valid time
	TxAt     *int64 // optional: filter by transaction time
	TxSeqAt  *int64 // optional: filter by write sequence instead of TxAt
}

// ensureFactsTable creates the facts table and indexes. Idempotent.
func (c *CatalogDB) ensureFactsTable() error {
	createSQL := `
	CREATE TABLE IF NOT EXISTS facts (
		id          TEXT PRIMARY KEY,
		relation    TEXT NOT NULL,
		args        TEXT NOT NULL,
		valid_start INTEGER NOT NULL,
		valid_end   INTEGER,
		tx_start    INTEGER NOT NULL,
		tx_end      INTEGER,
		source      TEXT,
		provenance  TEXT
	);`

	if _, err := c.db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create facts table: %w", err)
	}

	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_facts_relation ON facts(relation);",
		"CREATE INDEX IF NOT EXISTS idx_facts_valid ON facts(relation, valid_start, valid_end);",
		"CREATE INDEX IF NOT EXISTS idx_facts_tx ON facts(tx_start, tx_end);",
	}
	for _, idx := range indexes {
		if _, err := c.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create facts index: %w", err)
		}
	}

	return nil
}

// AddFact inserts a new fact into the store.
// The fact ID is computed from content if not already set.
// TxStart is set to now if zero.
// Also inserts into current_facts if the fact is currently valid (no valid_end or tx_end).
func (c *CatalogDB) AddFact(f Fact) (Fact, error) {
	tx, err := c.db.Begin()
	if err != nil {
		return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	f, err = addFactIn(tx, f)
	if err != nil {
		return Fact{}, err
	}

	if err := tx.Commit(); err != nil {
		return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to commit", err)
	}

	return f, nil
}

// addFactIn validates a fact, fills defaults, and inserts it (plus its
// current_facts row) on q. The caller owns the transaction boundary.
func addFactIn(q dbtx, f Fact) (Fact, error) {
	if f.Relation == "" {
		return Fact{}, errors.WrapError(errors.ErrCodeInvalidInput, "fact relation is required", nil)
	}
	if IsReservedRelation(f.Relation) {
		return Fact{}, errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("relation %q is reserved for the built-in catalog EDB and cannot be used for facts", f.Relation), nil)
	}
	if f.Args == "" {
		return Fact{}, errors.WrapError(errors.ErrCodeInvalidInput, "fact args is required", nil)
	}

	seq, err := allocFactSeq(q)
	if err != nil {
		return Fact{}, err
	}
	now := time.Now().Unix()

	f.TxSeq = seq
	f.TxEndSeq = nil
	if f.TxEnd != nil {
		f.TxEndSeq = &seq // recorded already closed
	}
	f.Supersedes = ""

	if f.ValidStart == 0 {
		f.ValidStart = now
	}
	if f.TxStart == 0 {
		f.TxStart = now
	}
	if f.ID == "" {
		f.ID = ComputeFactID(f.Relation, f.Args, f.ValidStart, f.Source)
	}

	// INSERT OR IGNORE: facts are content-addressed by ID, so re-adding an
	// identical fact is a no-op (natural deduplication), making replays and
	// imports idempotent.
	result, err := q.Exec(
		`INSERT OR IGNORE INTO facts (id, relation, args, valid_start, valid_end, tx_start, tx_end,
			source, provenance, tx_seq, tx_end_seq)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.Relation, f.Args, f.ValidStart, f.ValidEnd,
		f.TxStart, f.TxEnd, f.Source, f.Provenance, f.TxSeq, f.TxEndSeq)
	if err != nil {
		return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to add fact", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to read fact insert result", err)
	}
	if inserted == 0 {
		// A replay must project the stored fact, not the caller's stale copy.
		// In particular, terminal bounds and original provenance survive dedup.
		stored, err := scanFact(q.QueryRow(selectFactSQL+` WHERE id = ?`, f.ID))
		if err != nil {
			return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to read deduplicated fact", err)
		}
		if stored.Relation != f.Relation || stored.ValidStart != f.ValidStart || stored.Source != f.Source ||
			canonicalizeJSON(stored.Args) != canonicalizeJSON(f.Args) {
			return Fact{}, errors.WrapError(errors.ErrCodeInvalidInput,
				fmt.Sprintf("fact ID %s conflicts with stored content; preserve the stored fact and review the incoming evidence", f.ID), nil)
		}
		f = *stored
	}

	if f.ValidEnd == nil && f.TxEnd == nil {
		if err := insertCurrentFact(q, f); err != nil {
			return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to update current_facts", err)
		}
	} else if inserted == 0 {
		// Also repair a stale materialized row left by an older replay.
		if err := deleteCurrentFact(q, f.ID); err != nil {
			return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to remove terminal current fact", err)
		}
	}

	return f, nil
}

// RetractFact marks a fact as retracted by setting tx_end to now.
// Facts are never deleted — retraction preserves the full audit trail.
// An ID superseded by invalidation retracts its newest version.
// Also removes from current_facts.
func (c *CatalogDB) RetractFact(id string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	if err := retractFactIn(tx, id); err != nil {
		return err
	}

	return tx.Commit()
}

// retractFactIn closes the belief in the newest version of id's fact and
// removes its current_facts row on q. The caller owns the transaction boundary.
func retractFactIn(q dbtx, id string) error {
	seq, err := allocFactSeq(q)
	if err != nil {
		return err
	}
	latest, err := latestFactVersionIDIn(q, id)
	if err != nil {
		return err
	}
	now := time.Now().Unix()

	result, err := q.Exec(
		"UPDATE facts SET tx_end = ?, tx_end_seq = ? WHERE id = ? AND tx_end IS NULL",
		now, seq, latest)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to retract fact", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to get rows affected", err)
	}
	if rows == 0 {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("fact not found or already retracted: %s", id), nil)
	}

	if err := deleteCurrentFact(q, latest); err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to update current_facts", err)
	}

	return nil
}

// InvalidateFact records that a fact stopped being true now. This is distinct
// from retraction: the fact was true but is no longer.
//
// History is not rewritten. The open version's belief is closed (tx_end) and a
// successor version with valid_end set is recorded in the same transaction;
// the successor's ID is SupersededFactID(old ID, valid_end) and its Supersedes
// field names the old version. An ID that was already superseded resolves to
// its newest version, which is then already invalidated.
// Also removes from current_facts.
func (c *CatalogDB) InvalidateFact(id string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	if err := invalidateFactIn(tx, id); err != nil {
		return err
	}

	return tx.Commit()
}

// invalidateFactIn supersedes the open version of id's fact with a
// valid-time-bounded successor on q. The caller owns the transaction boundary.
func invalidateFactIn(q dbtx, id string) error {
	seq, err := allocFactSeq(q)
	if err != nil {
		return err
	}
	latest, err := latestFactVersionIDIn(q, id)
	if err != nil {
		return err
	}
	open, err := scanFact(q.QueryRow(selectFactSQL+` WHERE id = ?`, latest))
	if err == sql.ErrNoRows {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("fact not found: %s", id), nil)
	}
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to read fact", err)
	}
	if open.ValidEnd != nil {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("fact already invalidated: %s", id), nil)
	}
	if open.TxEnd != nil {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("fact is retracted, nothing to invalidate: %s", id), nil)
	}

	now := time.Now().Unix()
	if _, err := q.Exec(
		"UPDATE facts SET tx_end = ?, tx_end_seq = ? WHERE id = ? AND tx_end IS NULL",
		now, seq, open.ID); err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to close superseded fact", err)
	}

	successor := *open
	successor.ID = SupersededFactID(open.ID, now)
	successor.ValidEnd = &now
	successor.TxStart = now
	successor.TxSeq = seq
	successor.Supersedes = open.ID
	if _, err := q.Exec(
		`INSERT INTO facts (id, relation, args, valid_start, valid_end, tx_start, tx_end,
			source, provenance, tx_seq, tx_end_seq, supersedes)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, NULL, ?)`,
		successor.ID, successor.Relation, successor.Args, successor.ValidStart, successor.ValidEnd,
		successor.TxStart, successor.Source, successor.Provenance, successor.TxSeq, successor.Supersedes); err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to record invalidated fact", err)
	}

	if err := deleteCurrentFact(q, open.ID); err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to update current_facts", err)
	}

	return nil
}

// GetFact retrieves the exact stored version with this ID. A version superseded
// by invalidation is returned as recorded; LatestFactVersion follows the chain.
func (c *CatalogDB) GetFact(id string) (*Fact, error) {
	f, err := scanFact(c.db.QueryRow(selectFactSQL+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("fact not found: %s", id), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to get fact", err)
	}
	return f, nil
}

// GetFactByPrefix retrieves a single fact by ID prefix.
// Returns an error if the prefix is ambiguous (matches multiple facts).
func (c *CatalogDB) GetFactByPrefix(prefix string) (*Fact, error) {
	rows, err := c.db.Query(selectFactSQL+` WHERE id LIKE ? LIMIT 2`, prefix+"%")
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query by prefix", err)
	}
	defer rows.Close()

	facts, err := scanFactRows(rows)
	if err != nil {
		return nil, err
	}

	if len(facts) == 0 {
		return nil, errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("no fact found with prefix: %s", prefix), nil)
	}
	if len(facts) > 1 {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("ambiguous prefix %s: matches %s, %s, ...", prefix, facts[0].ID[:16], facts[1].ID[:16]), nil)
	}

	return &facts[0], nil
}

// QueryFacts returns facts matching the filter with bitemporal scoping.
func (c *CatalogDB) QueryFacts(filter FactFilter) ([]Fact, error) {
	return queryFactsIn(c.db, filter)
}

// queryFactsIn runs the bitemporal fact query on q.
func queryFactsIn(q dbtx, filter FactFilter) ([]Fact, error) {
	if filter.Relation == "" {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "fact filter requires a relation", nil)
	}

	if filter.TxAt != nil && filter.TxSeqAt != nil {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "fact filter accepts TxAt or TxSeqAt, not both", nil)
	}

	var conditions []string
	var args []interface{}

	conditions = append(conditions, "relation = ?")
	args = append(args, filter.Relation)

	// Scoping by write sequence is exact; it stands in for TxAt.
	if filter.TxSeqAt != nil {
		conditions = append(conditions, "tx_seq <= ?", "(tx_end_seq IS NULL OR tx_end_seq > ?)")
		args = append(args, *filter.TxSeqAt, *filter.TxSeqAt)
		if filter.ValidAt != nil {
			conditions = append(conditions, "valid_start <= ?", "(valid_end IS NULL OR valid_end > ?)")
			args = append(args, *filter.ValidAt, *filter.ValidAt)
		}
		return runFactQuery(q, conditions, args)
	}

	// Temporal scoping
	switch {
	case filter.ValidAt == nil && filter.TxAt == nil:
		// AsOfNow: currently valid, not retracted
		conditions = append(conditions, "valid_end IS NULL")
		conditions = append(conditions, "tx_end IS NULL")

	case filter.ValidAt != nil && filter.TxAt == nil:
		// AsOfValid: true at ValidAt, current knowledge
		conditions = append(conditions, "valid_start <= ?")
		args = append(args, *filter.ValidAt)
		conditions = append(conditions, "(valid_end IS NULL OR valid_end > ?)")
		args = append(args, *filter.ValidAt)
		conditions = append(conditions, "tx_end IS NULL")

	case filter.ValidAt == nil && filter.TxAt != nil:
		// AsOfTransaction: what we believed at TxAt
		conditions = append(conditions, "tx_start <= ?")
		args = append(args, *filter.TxAt)
		conditions = append(conditions, "(tx_end IS NULL OR tx_end > ?)")
		args = append(args, *filter.TxAt)

	default:
		// AsOf: what we believed at TxAt about what was true at ValidAt
		conditions = append(conditions, "valid_start <= ?")
		args = append(args, *filter.ValidAt)
		conditions = append(conditions, "(valid_end IS NULL OR valid_end > ?)")
		args = append(args, *filter.ValidAt)
		conditions = append(conditions, "tx_start <= ?")
		args = append(args, *filter.TxAt)
		conditions = append(conditions, "(tx_end IS NULL OR tx_end > ?)")
		args = append(args, *filter.TxAt)
	}

	return runFactQuery(q, conditions, args)
}

// runFactQuery selects facts matching every condition, newest valid time
// first and, within one valid_start, the latest write first.
func runFactQuery(q dbtx, conditions []string, args []interface{}) ([]Fact, error) {
	query := fmt.Sprintf(`%s WHERE %s ORDER BY valid_start DESC, tx_seq DESC`,
		selectFactSQL, strings.Join(conditions, " AND "))

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query facts", err)
	}
	defer rows.Close()

	return scanFactRows(rows)
}

// GetDistinctRelations returns all distinct relation names from current facts.
func (c *CatalogDB) GetDistinctRelations() ([]string, error) {
	rows, err := c.db.Query("SELECT DISTINCT relation FROM current_facts ORDER BY relation")
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query distinct relations", err)
	}
	defer rows.Close()

	var relations []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan relation", err)
		}
		relations = append(relations, r)
	}
	return relations, rows.Err()
}

// GetDistinctSources returns all distinct source values from current facts.
func (c *CatalogDB) GetDistinctSources() ([]string, error) {
	rows, err := c.db.Query("SELECT DISTINCT source FROM current_facts WHERE source IS NOT NULL AND source != '' ORDER BY source")
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query distinct sources", err)
	}
	defer rows.Close()

	var sources []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan source", err)
		}
		sources = append(sources, s)
	}
	return sources, rows.Err()
}

// scanFact scans a single fact from a sql.Row produced by selectFactSQL.
func scanFact(row *sql.Row) (*Fact, error) {
	return scanFactFrom(row)
}

// scanFactFrom reads the selectFactSQL columns, handling the nullable ones.
func scanFactFrom(row rowScanner) (*Fact, error) {
	var f Fact
	var validEnd, txEnd, txSeq, txEndSeq sql.NullInt64
	var source, provenance, supersedes sql.NullString

	err := row.Scan(&f.ID, &f.Relation, &f.Args,
		&f.ValidStart, &validEnd, &f.TxStart, &txEnd,
		&source, &provenance, &txSeq, &txEndSeq, &supersedes)
	if err != nil {
		return nil, err
	}

	if validEnd.Valid {
		f.ValidEnd = &validEnd.Int64
	}
	if txEnd.Valid {
		f.TxEnd = &txEnd.Int64
	}
	f.Source = source.String
	f.Provenance = provenance.String
	f.TxSeq = txSeq.Int64
	if txEndSeq.Valid {
		f.TxEndSeq = &txEndSeq.Int64
	}
	f.Supersedes = supersedes.String

	return &f, nil
}

// FactHistory returns every fact version ever recorded for a relation —
// including those since retracted or superseded (tx_end set) and invalidated
// successors (valid_end set) — ordered by write sequence. This is the audit
// trail; QueryFacts only sees live facts.
func (c *CatalogDB) FactHistory(relation string) ([]Fact, error) {
	return factHistoryIn(c.db, relation)
}

// factHistoryIn runs the fact history query on q.
func factHistoryIn(q dbtx, relation string) ([]Fact, error) {
	if relation == "" {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "fact history requires a relation", nil)
	}
	rows, err := q.Query(selectFactSQL+` WHERE relation = ? ORDER BY tx_seq ASC`, relation)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query fact history", err)
	}
	defer rows.Close()

	return scanFactRows(rows)
}

// scanFactRows collects facts from rows produced by selectFactSQL. The result
// is never nil, so an empty match encodes as a JSON array.
func scanFactRows(rows *sql.Rows) ([]Fact, error) {
	facts := []Fact{}
	for rows.Next() {
		f, err := scanFactFrom(rows)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan fact", err)
		}
		facts = append(facts, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "error iterating facts", err)
	}
	return facts, nil
}

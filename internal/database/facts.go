package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pudl/internal/errors"
)

// Fact represents a single fact in the bitemporal fact store.
// Facts are the EDB for the Datalog evaluator and the storage layer
// for agent observations.
type Fact struct {
	ID         string  `json:"id"`
	Relation   string  `json:"relation"`
	Args       string  `json:"args"`        // JSON object with meaningful keys
	ValidStart int64   `json:"valid_start"` // unix timestamp
	ValidEnd   *int64  `json:"valid_end,omitempty"`
	TxStart    int64   `json:"tx_start"`
	TxEnd      *int64  `json:"tx_end,omitempty"`
	Source     string  `json:"source,omitempty"`
	Provenance string  `json:"provenance,omitempty"` // JSON
}

// FactFilter specifies criteria for querying facts.
// ValidAt and TxAt control temporal query mode:
//   - both nil:       AsOfNow (current valid, current tx)
//   - ValidAt set:    AsOfValid (what was true at ValidAt, current knowledge)
//   - TxAt set:       AsOfTransaction (what we believed at TxAt)
//   - both set:       AsOf (what we believed at TxAt about what was true at ValidAt)
type FactFilter struct {
	Relation string // required
	ValidAt  *int64 // optional: filter by valid time
	TxAt     *int64 // optional: filter by transaction time
}

// ComputeFactID produces a content-addressed ID for a fact.
// ID = SHA256(relation + "\x00" + canonical_args + "\x00" + valid_start + "\x00" + source)
func ComputeFactID(relation, args string, validStart int64, source string) string {
	// Canonicalize args JSON to ensure consistent hashing
	canonical := canonicalizeJSON(args)
	payload := fmt.Sprintf("%s\x00%s\x00%d\x00%s", relation, canonical, validStart, source)
	hash := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", hash)
}

// canonicalizeJSON re-serializes JSON with sorted keys for consistent hashing.
func canonicalizeJSON(raw string) string {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return raw // not valid JSON object, use as-is
	}
	canonical, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return string(canonical)
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
func (c *CatalogDB) AddFact(f Fact) (Fact, error) {
	if f.Relation == "" {
		return Fact{}, errors.WrapError(errors.ErrCodeInvalidInput, "fact relation is required", nil)
	}
	if f.Args == "" {
		return Fact{}, errors.WrapError(errors.ErrCodeInvalidInput, "fact args is required", nil)
	}

	now := time.Now().Unix()

	if f.ValidStart == 0 {
		f.ValidStart = now
	}
	if f.TxStart == 0 {
		f.TxStart = now
	}
	if f.ID == "" {
		f.ID = ComputeFactID(f.Relation, f.Args, f.ValidStart, f.Source)
	}

	insertSQL := `
	INSERT INTO facts (id, relation, args, valid_start, valid_end, tx_start, tx_end, source, provenance)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := c.db.Exec(insertSQL,
		f.ID, f.Relation, f.Args, f.ValidStart, f.ValidEnd,
		f.TxStart, f.TxEnd, f.Source, f.Provenance)
	if err != nil {
		return Fact{}, errors.WrapError(errors.ErrCodeDatabaseError, "failed to add fact", err)
	}

	return f, nil
}

// RetractFact marks a fact as retracted by setting tx_end to now.
// Facts are never deleted — retraction preserves the full audit trail.
func (c *CatalogDB) RetractFact(id string) error {
	now := time.Now().Unix()

	result, err := c.db.Exec(
		"UPDATE facts SET tx_end = ? WHERE id = ? AND tx_end IS NULL",
		now, id)
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

	return nil
}

// InvalidateFact marks a fact as no longer valid by setting valid_end to now.
// This is distinct from retraction: the fact was true but is no longer.
func (c *CatalogDB) InvalidateFact(id string) error {
	now := time.Now().Unix()

	result, err := c.db.Exec(
		"UPDATE facts SET valid_end = ? WHERE id = ? AND valid_end IS NULL",
		now, id)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to invalidate fact", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to get rows affected", err)
	}
	if rows == 0 {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("fact not found or already invalidated: %s", id), nil)
	}

	return nil
}

// GetFact retrieves a single fact by ID.
func (c *CatalogDB) GetFact(id string) (*Fact, error) {
	row := c.db.QueryRow(
		`SELECT id, relation, args, valid_start, valid_end, tx_start, tx_end, source, provenance
		 FROM facts WHERE id = ?`, id)

	f, err := scanFact(row)
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
	rows, err := c.db.Query(
		`SELECT id, relation, args, valid_start, valid_end, tx_start, tx_end, source, provenance
		 FROM facts WHERE id LIKE ? LIMIT 2`, prefix+"%")
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query by prefix", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		var validEnd, txEnd sql.NullInt64
		var source, provenance sql.NullString
		err := rows.Scan(&f.ID, &f.Relation, &f.Args,
			&f.ValidStart, &validEnd, &f.TxStart, &txEnd,
			&source, &provenance)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan fact", err)
		}
		if validEnd.Valid {
			f.ValidEnd = &validEnd.Int64
		}
		if txEnd.Valid {
			f.TxEnd = &txEnd.Int64
		}
		if source.Valid {
			f.Source = source.String
		}
		if provenance.Valid {
			f.Provenance = provenance.String
		}
		facts = append(facts, f)
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
	if filter.Relation == "" {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "fact filter requires a relation", nil)
	}

	var conditions []string
	var args []interface{}

	conditions = append(conditions, "relation = ?")
	args = append(args, filter.Relation)

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

	query := fmt.Sprintf(
		`SELECT id, relation, args, valid_start, valid_end, tx_start, tx_end, source, provenance
		 FROM facts WHERE %s ORDER BY valid_start DESC`,
		strings.Join(conditions, " AND "))

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query facts", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		var validEnd, txEnd sql.NullInt64
		var source, provenance sql.NullString

		err := rows.Scan(&f.ID, &f.Relation, &f.Args,
			&f.ValidStart, &validEnd, &f.TxStart, &txEnd,
			&source, &provenance)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan fact", err)
		}

		if validEnd.Valid {
			f.ValidEnd = &validEnd.Int64
		}
		if txEnd.Valid {
			f.TxEnd = &txEnd.Int64
		}
		if source.Valid {
			f.Source = source.String
		}
		if provenance.Valid {
			f.Provenance = provenance.String
		}

		facts = append(facts, f)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "error iterating facts", err)
	}

	return facts, nil
}

// scanFact scans a single fact from a sql.Row.
func scanFact(row *sql.Row) (*Fact, error) {
	var f Fact
	var validEnd, txEnd sql.NullInt64
	var source, provenance sql.NullString

	err := row.Scan(&f.ID, &f.Relation, &f.Args,
		&f.ValidStart, &validEnd, &f.TxStart, &txEnd,
		&source, &provenance)
	if err != nil {
		return nil, err
	}

	if validEnd.Valid {
		f.ValidEnd = &validEnd.Int64
	}
	if txEnd.Valid {
		f.TxEnd = &txEnd.Int64
	}
	if source.Valid {
		f.Source = source.String
	}
	if provenance.Valid {
		f.Provenance = provenance.String
	}

	return &f, nil
}

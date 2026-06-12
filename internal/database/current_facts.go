package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/errors"
)

// ensureCurrentFactsTable creates the materialized current-state view of facts.
// Only holds facts that are currently valid and not retracted (valid_end IS NULL AND tx_end IS NULL).
// Kept in sync transactionally by AddFact, RetractFact, and InvalidateFact.
func (c *CatalogDB) ensureCurrentFactsTable() error {
	createSQL := `
	CREATE TABLE IF NOT EXISTS current_facts (
		id          TEXT PRIMARY KEY,
		relation    TEXT NOT NULL,
		args        TEXT NOT NULL,
		source      TEXT,
		provenance  TEXT
	);`

	if _, err := c.db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create current_facts table: %w", err)
	}

	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_current_facts_relation ON current_facts(relation);",
	}
	for _, idx := range indexes {
		if _, err := c.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create current_facts index: %w", err)
		}
	}

	return nil
}

// backfillCurrentFacts populates current_facts from the facts table.
// Only runs when current_facts is empty (first migration).
func (c *CatalogDB) backfillCurrentFacts() error {
	var count int
	if err := c.db.QueryRow("SELECT COUNT(*) FROM current_facts").Scan(&count); err != nil {
		return fmt.Errorf("failed to count current_facts: %w", err)
	}
	if count > 0 {
		return nil
	}

	_, err := c.db.Exec(`
		INSERT INTO current_facts (id, relation, args, source, provenance)
		SELECT id, relation, args, source, provenance
		FROM facts
		WHERE valid_end IS NULL AND tx_end IS NULL`)
	return err
}

// QueryCurrentFacts returns currently-valid facts for a relation.
// Faster than QueryFacts with AsOfNow because it avoids temporal filtering.
func (c *CatalogDB) QueryCurrentFacts(relation string) ([]Fact, error) {
	if relation == "" {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "relation is required", nil)
	}

	rows, err := c.db.Query(
		`SELECT id, relation, args, source, provenance FROM current_facts WHERE relation = ?`,
		relation)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query current facts", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		var source, provenance sql.NullString

		if err := rows.Scan(&f.ID, &f.Relation, &f.Args, &source, &provenance); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan current fact", err)
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
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "error iterating current facts", err)
	}

	return facts, nil
}

// QueryCurrentFactsFiltered returns currently-valid facts matching relation and arg constraints.
func (c *CatalogDB) QueryCurrentFactsFiltered(relation string, argFilters map[string]interface{}) ([]Fact, error) {
	if relation == "" {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "relation is required", nil)
	}

	conditions := []string{"relation = ?"}
	args := []interface{}{relation}

	for key, val := range argFilters {
		conditions = append(conditions, "json_extract(args, ?) = ?")
		args = append(args, "$."+key, val)
	}

	query := fmt.Sprintf(
		`SELECT id, relation, args, source, provenance FROM current_facts WHERE %s`,
		strings.Join(conditions, " AND "))

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to query current facts", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		var source, provenance sql.NullString

		if err := rows.Scan(&f.ID, &f.Relation, &f.Args, &source, &provenance); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan current fact", err)
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
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "error iterating current facts", err)
	}

	return facts, nil
}

// ListCurrentRelations returns all distinct relation names in current_facts.
func (c *CatalogDB) ListCurrentRelations() ([]string, error) {
	rows, err := c.db.Query("SELECT DISTINCT relation FROM current_facts ORDER BY relation")
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to list relations", err)
	}
	defer rows.Close()

	var relations []string
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan relation", err)
		}
		relations = append(relations, rel)
	}
	return relations, rows.Err()
}

// CountCurrentFacts returns the number of currently-valid facts, optionally filtered by relation.
func (c *CatalogDB) CountCurrentFacts(relation string) (int, error) {
	var count int
	var err error
	if relation == "" {
		err = c.db.QueryRow("SELECT COUNT(*) FROM current_facts").Scan(&count)
	} else {
		err = c.db.QueryRow("SELECT COUNT(*) FROM current_facts WHERE relation = ?", relation).Scan(&count)
	}
	if err != nil {
		return 0, errors.WrapError(errors.ErrCodeDatabaseError, "failed to count current facts", err)
	}
	return count, nil
}

// insertCurrentFact adds a fact to the current_facts materialized view.
func insertCurrentFact(q dbtx, f Fact) error {
	_, err := q.Exec(
		`INSERT OR REPLACE INTO current_facts (id, relation, args, source, provenance)
		 VALUES (?, ?, ?, ?, ?)`,
		f.ID, f.Relation, f.Args, f.Source, f.Provenance)
	return err
}

// deleteCurrentFact removes a fact from the current_facts materialized view.
func deleteCurrentFact(q dbtx, id string) error {
	_, err := q.Exec("DELETE FROM current_facts WHERE id = ?", id)
	return err
}

// canonicalArgs re-serializes args JSON for consistent comparison.
func canonicalArgs(raw string) string {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return raw
	}
	canonical, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return string(canonical)
}

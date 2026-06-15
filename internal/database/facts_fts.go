package database

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/errors"
)

// FactsFTSTable is the FTS5 virtual table providing keyword search over the
// currently-valid facts. It indexes the textual values of each fact's args (not
// the JSON keys), kept in sync with current_facts by insertCurrentFact and
// deleteCurrentFact.
const FactsFTSTable = "current_facts_fts"

// ensureFactsFTSTable creates the FTS5 search index over current facts and
// backfills it from current_facts when empty. Must run after current_facts
// exists and is backfilled.
func (c *CatalogDB) ensureFactsFTSTable() error {
	createSQL := fmt.Sprintf(
		`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(id UNINDEXED, relation UNINDEXED, text)`,
		FactsFTSTable)
	if _, err := c.db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create %s: %w", FactsFTSTable, err)
	}

	var count int
	if err := c.db.QueryRow("SELECT COUNT(*) FROM " + FactsFTSTable).Scan(&count); err != nil {
		return fmt.Errorf("failed to count %s: %w", FactsFTSTable, err)
	}
	if count > 0 {
		return nil
	}

	rows, err := c.db.Query("SELECT id, relation, args FROM current_facts")
	if err != nil {
		return fmt.Errorf("failed to read current_facts for FTS backfill: %w", err)
	}
	defer rows.Close()

	type rec struct{ id, relation, args string }
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.relation, &r.args); err != nil {
			return fmt.Errorf("scan current_facts for FTS backfill: %w", err)
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range recs {
		if _, err := c.db.Exec(
			fmt.Sprintf("INSERT INTO %s(id, relation, text) VALUES (?, ?, ?)", FactsFTSTable),
			r.id, r.relation, factSearchText(r.args)); err != nil {
			return fmt.Errorf("FTS backfill insert: %w", err)
		}
	}
	return nil
}

// syncFactFTS upserts a fact's search row (delete-then-insert, since FTS5 has no
// REPLACE). Runs on the same executor as the current_facts write so it shares the
// transaction.
func syncFactFTS(q dbtx, f Fact) error {
	if _, err := q.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", FactsFTSTable), f.ID); err != nil {
		return err
	}
	_, err := q.Exec(
		fmt.Sprintf("INSERT INTO %s(id, relation, text) VALUES (?, ?, ?)", FactsFTSTable),
		f.ID, f.Relation, factSearchText(f.Args))
	return err
}

// deleteFactFTS removes a fact's search row.
func deleteFactFTS(q dbtx, id string) error {
	_, err := q.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", FactsFTSTable), id)
	return err
}

// factSearchText extracts the textual content to index from a fact's args JSON:
// the string and scalar values (not the keys), space-joined. Falls back to the
// raw args when it is not a JSON object.
func factSearchText(argsJSON string) string {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &obj); err != nil {
		return argsJSON
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		switch v := obj[k].(type) {
		case string:
			parts = append(parts, v)
		case bool:
			parts = append(parts, fmt.Sprintf("%t", v))
		case float64:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	return strings.Join(parts, " ")
}

// SearchCurrentFacts returns currently-valid facts whose indexed text matches the
// FTS5 query, best matches first (bm25 rank). An optional relation filter narrows
// the result; limit <= 0 means no limit.
func (c *CatalogDB) SearchCurrentFacts(query, relation string, limit int) ([]Fact, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "search query is required", nil)
	}

	// Join facts for valid_start/tx_start so returned Facts carry correct
	// timestamps (current_facts has none).
	sqlStr := fmt.Sprintf(`SELECT cf.id, cf.relation, cf.args, cf.source, cf.provenance, f.valid_start, f.tx_start
		FROM %s ft
		JOIN current_facts cf ON cf.id = ft.id
		JOIN facts f ON f.id = cf.id
		WHERE ft.text MATCH ?`, FactsFTSTable)
	params := []interface{}{query}
	if relation != "" {
		sqlStr += " AND cf.relation = ?"
		params = append(params, relation)
	}
	sqlStr += " ORDER BY ft.rank"
	if limit > 0 {
		sqlStr += " LIMIT ?"
		params = append(params, limit)
	}

	rows, err := c.db.Query(sqlStr, params...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "fact search failed", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		var source, provenance interface{}
		if err := rows.Scan(&f.ID, &f.Relation, &f.Args, &source, &provenance, &f.ValidStart, &f.TxStart); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan search result", err)
		}
		if s, ok := source.(string); ok {
			f.Source = s
		}
		if p, ok := provenance.(string); ok {
			f.Provenance = p
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

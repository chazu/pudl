package database

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/errors"
)

// MemoryContextItem is a promoted observation surfaced for agent context, with
// its read-time decay score.
type MemoryContextItem struct {
	Fact
	DecayedWorth float64
}

// MemoryContext returns promoted observations ranked for injection into an
// agent's context: when task is non-empty it keyword-matches (FTS5) and ranks by
// decayed worth; otherwise it returns the highest-decayed-worth promoted
// observations. This is the read side of the recall loop (the "Generator"); it
// performs no model calls. limit <= 0 returns all matches.
func (c *CatalogDB) MemoryContext(task string, limit int) ([]MemoryContextItem, error) {
	var (
		query  string
		params []interface{}
	)

	if strings.TrimSpace(task) != "" {
		query = fmt.Sprintf(`SELECT cf.id, cf.relation, cf.args, cf.source, cf.provenance, fs.decayed_worth
			FROM %s ft
			JOIN current_facts cf ON cf.id = ft.id
			JOIN %s fs ON fs.id = cf.id
			WHERE ft.text MATCH ?
			  AND cf.relation = 'observation'
			  AND json_extract(cf.args, '$.status') = 'promoted'
			ORDER BY fs.decayed_worth DESC`, FactsFTSTable, FactScoredView)
		params = append(params, task)
	} else {
		query = fmt.Sprintf(`SELECT cf.id, cf.relation, cf.args, cf.source, cf.provenance, fs.decayed_worth
			FROM %s fs
			JOIN current_facts cf ON cf.id = fs.id
			WHERE cf.relation = 'observation'
			  AND json_extract(cf.args, '$.status') = 'promoted'
			ORDER BY fs.decayed_worth DESC`, FactScoredView)
	}
	if limit > 0 {
		query += " LIMIT ?"
		params = append(params, limit)
	}

	rows, err := c.db.Query(query, params...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "memory context query failed", err)
	}
	defer rows.Close()

	var items []MemoryContextItem
	for rows.Next() {
		var it MemoryContextItem
		var source, provenance interface{}
		var decayed interface{}
		if err := rows.Scan(&it.ID, &it.Relation, &it.Args, &source, &provenance, &decayed); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan memory context", err)
		}
		if s, ok := source.(string); ok {
			it.Source = s
		}
		if p, ok := provenance.(string); ok {
			it.Provenance = p
		}
		switch d := decayed.(type) {
		case float64:
			it.DecayedWorth = d
		case int64:
			it.DecayedWorth = float64(d)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

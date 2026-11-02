package database

import "fmt"

// SchemaCoverage counts one schema's catalog entries: how many resolved to a
// resource identity and how many did not.
type SchemaCoverage struct {
	Schema       string `json:"schema"`
	Entries      int    `json:"entries"`
	Unidentified int    `json:"unidentified"`
}

// SchemaCoverageCounts counts entries per schema, leaving out collection
// containers (an imported JSON array's wrapper). An entry is unidentified
// when its schema has no identity fields, or they did not resolve, so it has
// no resource_id and appears in no resource view.
func (c *CatalogDB) SchemaCoverageCounts() ([]SchemaCoverage, error) {
	rows, err := c.db.Query(`SELECT schema, COUNT(*),
			SUM(CASE WHEN resource_id IS NULL OR identity_json IS NULL OR identity_json = '' THEN 1 ELSE 0 END)
		FROM catalog_entries
		WHERE COALESCE(collection_type, '') != 'collection'
		GROUP BY schema ORDER BY schema`)
	if err != nil {
		return nil, fmt.Errorf("schema coverage: %w", err)
	}
	defer rows.Close()
	var out []SchemaCoverage
	for rows.Next() {
		var s SchemaCoverage
		if err := rows.Scan(&s.Schema, &s.Entries, &s.Unidentified); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

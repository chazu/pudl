package database

import "fmt"

// repairFactProjections repairs the materialized state written by old AddFact
// replays. It runs once as a data migration: historical facts and IDs are never
// rewritten, and the current rows plus their search index are replaced together.
func (c *CatalogDB) repairFactProjections() error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"current_facts", FactsFTSTable} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	rows, err := tx.Query(`SELECT id, relation, args, COALESCE(source, ''), COALESCE(provenance, '')
		FROM facts WHERE valid_end IS NULL AND tx_end IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	// Read from facts while writing only its projections. Keep memory bounded
	// by one fact rather than materializing the complete history for repair.
	for rows.Next() {
		var f Fact
		if err := rows.Scan(&f.ID, &f.Relation, &f.Args, &f.Source, &f.Provenance); err != nil {
			return err
		}
		if err := insertCurrentFact(tx, f); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return tx.Commit()
}

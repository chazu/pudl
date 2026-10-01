package database

import "fmt"

// Retire the derived scoring policy while preserving every historical fact.
func (c *CatalogDB) retireAgentMemoryView() error {
	if _, err := c.db.Exec("DROP VIEW IF EXISTS fact_scored_edb"); err != nil {
		return fmt.Errorf("retire agent memory scoring view: %w", err)
	}
	return nil
}

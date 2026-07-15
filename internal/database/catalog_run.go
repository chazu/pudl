package database

import "fmt"

// UpdateEntryRunID associates a deduplicated catalog entry with the current
// observation run. The content-addressed entry remains shared; snapshot
// membership is the historical relationship, while this field identifies the
// run that most recently observed the item.
func (c *CatalogDB) UpdateEntryRunID(entryID, runID string) error {
	if entryID == "" || runID == "" {
		return nil
	}
	result, err := c.db.Exec(
		`UPDATE catalog_entries SET run_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		runID, entryID,
	)
	if err != nil {
		return fmt.Errorf("update run_id for %q: %w", entryID, err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("check run_id update for %q: %w", entryID, err)
	} else if n == 0 {
		return fmt.Errorf("catalog entry %q not found", entryID)
	}
	return nil
}

package database

import (
	"database/sql"
	"fmt"

	"github.com/chazu/pudl/internal/errors"
)

// GetManifestActions returns all manifest-action entries for a given run_id.
func (c *CatalogDB) GetManifestActions(runID string) ([]CatalogEntry, error) {
	selectSQL := `
	SELECT ` + entrySelect("catalog_entries") + `
	FROM catalog_entries
	WHERE entry_type = 'manifest-action' AND run_id = ?
	ORDER BY import_timestamp ASC`

	rows, err := c.db.Query(selectSQL, runID)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query manifest actions", err)
	}
	defer rows.Close()

	var entries []CatalogEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan manifest action", err)
		}
		entries = append(entries, *entry)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating manifest actions", err)
	}

	return entries, nil
}

// GetLatestManifestAction returns the most recent manifest-action for a target.
func (c *CatalogDB) GetLatestManifestAction(targetName string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT ` + entrySelect("catalog_entries") + `
	FROM catalog_entries
	WHERE entry_type = 'manifest-action' AND target = ?
	ORDER BY import_timestamp DESC
	LIMIT 1`

	entry, err := scanEntry(c.db.QueryRow(selectSQL, targetName))
	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound,
			fmt.Sprintf("No manifest action found for target: %s", targetName), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get latest manifest action", err)
	}

	return entry, nil
}

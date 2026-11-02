package database

import (
	"database/sql"
	"fmt"

	"github.com/chazu/pudl/internal/errors"
)

// GetLatestObserve returns the most recent observe entry for a target.
func (c *CatalogDB) GetLatestObserve(targetName string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT ` + entrySelect("catalog_entries") + `
	FROM catalog_entries
	WHERE entry_type = 'observe' AND target = ?
	ORDER BY import_timestamp DESC
	LIMIT 1`

	entry, err := scanEntry(c.db.QueryRow(selectSQL, targetName))

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError,
			fmt.Sprintf("Failed to get latest observe for %s", targetName), err)
	}

	return entry, nil
}

// GetLatestObserveByOrigin returns the most recent observe entry for a target
// filtered by origin.
func (c *CatalogDB) GetLatestObserveByOrigin(targetName, origin string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT ` + entrySelect("catalog_entries") + `
	FROM catalog_entries
	WHERE entry_type = 'observe' AND target = ? AND origin = ?
	ORDER BY import_timestamp DESC
	LIMIT 1`

	var entry CatalogEntry
	err := c.db.QueryRow(selectSQL, targetName, origin).Scan(
		&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
		&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
		&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
		&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
		&entry.IdentityJSON, &entry.Version, &entry.EntryType, &entry.Target,
		&entry.RunID, &entry.Tags, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError,
			fmt.Sprintf("Failed to get latest observe by origin for %s", targetName), err)
	}

	return &entry, nil
}

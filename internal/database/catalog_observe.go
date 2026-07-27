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

// GetLatestObserveByContentHash checks if an observe entry with the given
// content hash already exists for a target.
func (c *CatalogDB) GetLatestObserveByContentHash(targetName, contentHash string) (*CatalogEntry, error) {
	return getLatestObserveByContentHashIn(c.db, targetName, contentHash)
}

// getLatestObserveByContentHashIn is the executor-parameterized form. Inside a
// transaction it also sees records this same step has already inserted, so a
// batch containing the same record twice deduplicates against itself.
func getLatestObserveByContentHashIn(q dbtx, targetName, contentHash string) (*CatalogEntry, error) {
	entry, err := scanOptionalEntry(q.QueryRow(
		`SELECT `+entrySelect("catalog_entries")+` FROM catalog_entries
		 WHERE entry_type = 'observe' AND target = ? AND content_hash = ?
		 LIMIT 1`,
		targetName, contentHash))
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to check observe dedup", err)
	}
	return entry, nil
}

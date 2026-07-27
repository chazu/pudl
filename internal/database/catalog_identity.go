package database

import (
	"database/sql"
	"fmt"

	"github.com/chazu/pudl/internal/errors"
)

// FindByContentHash returns entry with matching content hash, or nil.
func (c *CatalogDB) FindByContentHash(contentHash string) (*CatalogEntry, error) {
	return findByContentHashIn(c.db, contentHash)
}

// findByContentHashIn is the executor-parameterized form, so a manifest step can
// run its dedup check and its inserts in one transaction — the check and the
// write it guards cannot then interleave with another writer.
func findByContentHashIn(q dbtx, contentHash string) (*CatalogEntry, error) {
	entry, err := scanOptionalEntry(q.QueryRow(
		`SELECT `+entrySelect("catalog_entries")+` FROM catalog_entries WHERE content_hash = ? LIMIT 1`,
		contentHash))
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to find entry by content hash", err)
	}
	return entry, nil
}

// FindByResourceID returns all versions of a resource, newest first.
func (c *CatalogDB) FindByResourceID(resourceID string) ([]CatalogEntry, error) {
	selectSQL := `
	SELECT ` + entrySelect("catalog_entries") + `
	FROM catalog_entries
	WHERE resource_id = ?
	ORDER BY version DESC`

	rows, err := c.db.Query(selectSQL, resourceID)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to find entries by resource ID", err)
	}
	defer rows.Close()

	var entries []CatalogEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan entry", err)
		}
		entries = append(entries, *entry)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating entries", err)
	}

	return entries, nil
}

// GetLatestVersion returns the highest version number for a resource_id.
// Returns 0 if no entries exist.
func (c *CatalogDB) GetLatestVersion(resourceID string) (int, error) {
	var version sql.NullInt64
	err := c.db.QueryRow(
		"SELECT MAX(version) FROM catalog_entries WHERE resource_id = ?",
		resourceID,
	).Scan(&version)

	if err != nil {
		return 0, errors.WrapError(errors.ErrCodeDatabaseError,
			fmt.Sprintf("Failed to get latest version for resource %s", resourceID), err)
	}

	if !version.Valid {
		return 0, nil
	}

	return int(version.Int64), nil
}

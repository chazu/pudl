package database

import (
	"database/sql"
	"fmt"

	"pudl/internal/errors"
)

// FindByContentHash returns entry with matching content hash, or nil.
func (c *CatalogDB) FindByContentHash(contentHash string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   created_at, updated_at
	FROM catalog_entries
	WHERE content_hash = ?
	LIMIT 1`

	var entry CatalogEntry
	err := c.db.QueryRow(selectSQL, contentHash).Scan(
		&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
		&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
		&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
		&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
		&entry.IdentityJSON, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to find entry by content hash", err)
	}

	return &entry, nil
}

// FindByResourceID returns all versions of a resource, newest first.
func (c *CatalogDB) FindByResourceID(resourceID string) ([]CatalogEntry, error) {
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   created_at, updated_at
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
		var entry CatalogEntry
		err := rows.Scan(
			&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
			&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
			&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
			&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
			&entry.IdentityJSON, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan entry", err)
		}
		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
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

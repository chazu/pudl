package database

import (
	"database/sql"
	"fmt"

	"pudl/internal/errors"
)

// GetLatestArtifact returns the most recent artifact for a definition+method pair.
func (c *CatalogDB) GetLatestArtifact(definition, method string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   entry_type, definition, method, run_id, tags, status,
		   created_at, updated_at
	FROM catalog_entries
	WHERE entry_type = 'artifact' AND definition = ? AND method = ?
	ORDER BY import_timestamp DESC
	LIMIT 1`

	var entry CatalogEntry
	err := c.db.QueryRow(selectSQL, definition, method).Scan(
		&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
		&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
		&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
		&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
		&entry.IdentityJSON, &entry.Version, &entry.EntryType, &entry.Definition,
		&entry.Method, &entry.RunID, &entry.Tags, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound,
			fmt.Sprintf("No artifact found for %s.%s", definition, method), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get latest artifact", err)
	}

	return &entry, nil
}

// GetLatestArtifactByOrigin returns the most recent artifact for a definition+method pair
// filtered by origin.
func (c *CatalogDB) GetLatestArtifactByOrigin(definition, method, origin string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   entry_type, definition, method, run_id, tags, status,
		   created_at, updated_at
	FROM catalog_entries
	WHERE entry_type = 'artifact' AND definition = ? AND method = ? AND origin = ?
	ORDER BY import_timestamp DESC
	LIMIT 1`

	var entry CatalogEntry
	err := c.db.QueryRow(selectSQL, definition, method, origin).Scan(
		&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
		&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
		&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
		&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
		&entry.IdentityJSON, &entry.Version, &entry.EntryType, &entry.Definition,
		&entry.Method, &entry.RunID, &entry.Tags, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound,
			fmt.Sprintf("No artifact found for %s.%s with origin %s", definition, method, origin), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get latest artifact by origin", err)
	}

	return &entry, nil
}


package database

import (
	"database/sql"
	"fmt"
	"strings"

	"pudl/internal/errors"
)

// ArtifactFilters contains filtering criteria for artifact queries.
type ArtifactFilters struct {
	Definition string
	Method     string
	Limit      int
}

// GetLatestArtifact returns the most recent artifact for a definition+method pair.
func (c *CatalogDB) GetLatestArtifact(definition, method string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   entry_type, definition, method, run_id, tags,
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
		&entry.Method, &entry.RunID, &entry.Tags, &entry.CreatedAt, &entry.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound,
			fmt.Sprintf("No artifact found for %s.%s", definition, method), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get latest artifact", err)
	}

	return &entry, nil
}

// SearchArtifacts returns artifacts matching the given filters.
func (c *CatalogDB) SearchArtifacts(filters ArtifactFilters) ([]CatalogEntry, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "entry_type = 'artifact'")

	if filters.Definition != "" {
		conditions = append(conditions, "definition = ?")
		args = append(args, filters.Definition)
	}
	if filters.Method != "" {
		conditions = append(conditions, "method = ?")
		args = append(args, filters.Method)
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	limit := filters.Limit
	if limit <= 0 {
		limit = 50
	}

	selectSQL := fmt.Sprintf(`
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   entry_type, definition, method, run_id, tags,
		   created_at, updated_at
	FROM catalog_entries
	%s
	ORDER BY import_timestamp DESC
	LIMIT %d`, whereClause, limit)

	rows, err := c.db.Query(selectSQL, args...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to search artifacts", err)
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
			&entry.IdentityJSON, &entry.Version, &entry.EntryType, &entry.Definition,
			&entry.Method, &entry.RunID, &entry.Tags, &entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan artifact", err)
		}
		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating artifacts", err)
	}

	return entries, nil
}

package database

import (
	"database/sql"
	"strings"
)

// This file is the single place that knows the shape of a catalog_entries row.
//
// The column list and its matching Scan were previously written out at every
// read site — a dozen copies that all had to be edited together to add a column,
// with nothing but discipline keeping them aligned. Reads now name
// entryColumns and hand the row to scanEntry, so a new column is added in one
// place and every reader picks it up.

// entryColumns is the catalog_entries column list, in the order scanEntry
// expects. Keep the two in step: they are a pair.
const entryColumns = `id, stored_path, metadata_path, import_timestamp, format, origin,
	schema, confidence, record_count, size_bytes, collection_id, item_index,
	collection_type, item_id, resource_id, content_hash, identity_json, version,
	entry_type, target, run_id, tags, status,
	created_at, updated_at`

// prefixedEntryColumns is entryColumns qualified with a table alias, for reads
// that join catalog_entries against another table and would otherwise have
// ambiguous column names. Derived from the same constant, so a new column still
// only has to be added once.
func prefixedEntryColumns(alias string) string {
	columns := strings.Split(entryColumns, ",")
	for i, column := range columns {
		columns[i] = alias + "." + strings.TrimSpace(column)
	}
	return strings.Join(columns, ", ")
}

// rowScanner is the part of *sql.Row and *sql.Rows that scanEntry needs, so one
// mapping serves both single-row and multi-row reads.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanEntry maps one catalog_entries row selected with entryColumns.
func scanEntry(row rowScanner) (*CatalogEntry, error) {
	var entry CatalogEntry
	err := row.Scan(
		&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
		&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
		&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
		&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
		&entry.IdentityJSON, &entry.Version, &entry.EntryType, &entry.Target,
		&entry.RunID, &entry.Tags, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// scanOptionalEntry maps a single-row read where "no such row" is a normal
// answer rather than a failure, returning (nil, nil) for it.
func scanOptionalEntry(row rowScanner) (*CatalogEntry, error) {
	entry, err := scanEntry(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

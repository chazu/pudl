package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// This file is the single place that knows the shape of a catalog_entries row.
//
// The column list and its matching Scan were previously written out at every
// read site — a dozen copies that all had to be edited together to add a column,
// with nothing but discipline keeping them aligned. Reads now name
// entrySelect(...) and hand the row to scanEntry, so a new column is added in
// one place and every reader picks it up.

// storedColumns are the catalog_entries columns that physically exist, in the
// order scanEntry expects them *after* the two derived membership columns are
// spliced in. Keep this, membershipColumnIndex and scanEntry in step: they are a
// set.
var storedColumns = []string{
	"id", "stored_path", "metadata_path", "import_timestamp", "format", "origin",
	"schema", "confidence", "record_count", "size_bytes",
	// collection_id and item_index are derived here, not stored — see below.
	"collection_type", "item_id", "resource_id", "content_hash", "identity_json", "version",
	"entry_type", "target", "run_id", "tags", "status",
	"created_at", "updated_at",
}

// membershipColumnIndex is where collection_id and item_index are spliced into
// the select list: immediately after size_bytes, matching CatalogEntry's field
// order and scanEntry's argument order.
const membershipColumnIndex = 10

// derivedMembershipColumns answers "which collection is this item in?" from
// collection_memberships rather than from a column written once at insert.
//
// The HAVING guard is the point. An item is content-addressed and can belong to
// several collections — an observe record shared by three snapshots is the
// normal case — so there is no single collection ID to return. The legacy column
// returned one anyway: whichever collection happened to insert it first. Nil is
// the truthful answer, and every caller already nil-checks these fields, so a
// shared item simply omits its membership rather than naming an arbitrary one.
//
// A collection-scoped read overrides both with the collection it read through —
// see entrySelectVia.
func derivedMembershipColumns(alias string) []string {
	return []string{
		fmt.Sprintf(`(SELECT m.collection_id FROM collection_memberships m
			WHERE m.item_id = %s.id GROUP BY m.item_id HAVING COUNT(*) = 1) AS collection_id`, alias),
		fmt.Sprintf(`(SELECT m.item_index FROM collection_memberships m
			WHERE m.item_id = %s.id GROUP BY m.item_id HAVING COUNT(*) = 1) AS item_index`, alias),
	}
}

// entrySelect renders the canonical select list for a catalog_entries read.
//
// alias is the table alias to qualify columns with; pass "catalog_entries" for
// an unaliased single-table query (SQLite resolves the table name as its own
// alias), or the alias used in a join.
func entrySelect(alias string) string {
	return strings.Join(entryColumnExpressions(alias, derivedMembershipColumns(alias)), ", ")
}

// entrySelectVia is entrySelect for a read that already joins a specific
// membership row: the entry carries *that* collection and its index within it,
// which is the only well-defined answer for a shared item.
func entrySelectVia(entryAlias, membershipAlias string) string {
	return strings.Join(entryColumnExpressions(entryAlias, []string{
		membershipAlias + ".collection_id",
		membershipAlias + ".item_index",
	}), ", ")
}

func entryColumnExpressions(alias string, membership []string) []string {
	columns := make([]string, 0, len(storedColumns)+len(membership))
	for _, column := range storedColumns[:membershipColumnIndex] {
		columns = append(columns, alias+"."+column)
	}
	columns = append(columns, membership...)
	for _, column := range storedColumns[membershipColumnIndex:] {
		columns = append(columns, alias+"."+column)
	}
	return columns
}

// rowScanner is the part of *sql.Row and *sql.Rows that scanEntry needs, so one
// mapping serves both single-row and multi-row reads.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanEntry maps one catalog_entries row selected with entrySelect/entrySelectVia.
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

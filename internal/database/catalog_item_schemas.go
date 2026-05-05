package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ItemSchemaStatus enumerates the ways an item came to be associated
// with a schema reference. See plan
// docs/plans/2026-05-04-feat-plugin-output-schemas-plan.md (W4).
const (
	ItemSchemaStatusDeclared       = "declared"        // ref was known in the schema cache and the item was classified against it
	ItemSchemaStatusAutoRegistered = "auto_registered" // ref was registered from a vendored definition during import, then classified
	ItemSchemaStatusInferred       = "inferred"        // schema was determined by pudl inference, not by a declared ref
	ItemSchemaStatusUnresolved     = "unresolved"      // a ref was declared but could not be looked up; awaiting `pudl reclassify`
)

// ItemSchema is one row of the item_schemas junction table: a single
// (item, schema_ref, status) association.
type ItemSchema struct {
	ItemID        string
	SchemaRef     string // canonical form: "<module>@<version>" or "<module>@<version>#<definition>"
	Status        string
	ClassifiedAt  time.Time
}

// ensureItemSchemasTable creates the item_schemas table and indexes if
// they do not already exist. Idempotent.
func (c *CatalogDB) ensureItemSchemasTable() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS item_schemas (
    item_id        TEXT NOT NULL,
    schema_ref     TEXT NOT NULL,
    status         TEXT NOT NULL,
    classified_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (item_id, schema_ref)
);`
	if _, err := c.db.Exec(ddl); err != nil {
		return fmt.Errorf("create item_schemas: %w", err)
	}
	for _, idx := range []string{
		"CREATE INDEX IF NOT EXISTS idx_item_schemas_status ON item_schemas(status);",
		"CREATE INDEX IF NOT EXISTS idx_item_schemas_ref ON item_schemas(schema_ref);",
	} {
		if _, err := c.db.Exec(idx); err != nil {
			return fmt.Errorf("create item_schemas index: %w", err)
		}
	}
	return nil
}

// AddItemSchema inserts (or replaces) a single item_schemas row. The
// (item_id, schema_ref) pair is unique; re-inserting upgrades the
// status and refreshes classified_at.
func (c *CatalogDB) AddItemSchema(row ItemSchema) error {
	if row.ItemID == "" || row.SchemaRef == "" || row.Status == "" {
		return errors.New("item_schemas: item_id, schema_ref, and status are required")
	}
	if row.ClassifiedAt.IsZero() {
		row.ClassifiedAt = time.Now()
	}
	const q = `
INSERT INTO item_schemas (item_id, schema_ref, status, classified_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(item_id, schema_ref) DO UPDATE SET
    status        = excluded.status,
    classified_at = excluded.classified_at;`
	_, err := c.db.Exec(q, row.ItemID, row.SchemaRef, row.Status, row.ClassifiedAt)
	return err
}

// ListItemSchemas returns all schema rows for the given item, ordered
// by classified_at descending.
func (c *CatalogDB) ListItemSchemas(itemID string) ([]ItemSchema, error) {
	const q = `
SELECT item_id, schema_ref, status, classified_at
FROM item_schemas
WHERE item_id = ?
ORDER BY classified_at DESC;`
	return c.queryItemSchemas(q, itemID)
}

// ListUnresolvedItemSchemas returns all rows with status='unresolved'.
// If schemaRef is non-empty, results are filtered to that ref.
// Used by `pudl reclassify` to find rows awaiting a schema.
func (c *CatalogDB) ListUnresolvedItemSchemas(schemaRef string) ([]ItemSchema, error) {
	if schemaRef == "" {
		const q = `SELECT item_id, schema_ref, status, classified_at FROM item_schemas WHERE status = ? ORDER BY classified_at;`
		return c.queryItemSchemas(q, ItemSchemaStatusUnresolved)
	}
	const q = `SELECT item_id, schema_ref, status, classified_at FROM item_schemas WHERE status = ? AND schema_ref = ? ORDER BY classified_at;`
	return c.queryItemSchemas(q, ItemSchemaStatusUnresolved, schemaRef)
}

// DeleteItemSchema removes a single (item_id, schema_ref) row.
// Returns sql.ErrNoRows if no row matched.
func (c *CatalogDB) DeleteItemSchema(itemID, schemaRef string) error {
	res, err := c.db.Exec(`DELETE FROM item_schemas WHERE item_id = ? AND schema_ref = ?`, itemID, schemaRef)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (c *CatalogDB) queryItemSchemas(query string, args ...any) ([]ItemSchema, error) {
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemSchema
	for rows.Next() {
		var r ItemSchema
		if err := rows.Scan(&r.ItemID, &r.SchemaRef, &r.Status, &r.ClassifiedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

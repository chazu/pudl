package database

import "fmt"

// ensureItemSchemasTable keeps the legacy item_schemas table that the retired
// typed-envelope import wrote (declared, inferred and unresolved schema
// references per item). Nothing writes it any more; it stays so catalogs
// created before the removal open unchanged. Idempotent.
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

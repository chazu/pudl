package database

import (
	"fmt"
)

// ensureIdentityColumns adds identity tracking columns if missing.
// Idempotent — safe to run on every DB open.
func (c *CatalogDB) ensureIdentityColumns() error {
	columns := []struct {
		name string
		ddl  string
	}{
		{"resource_id", "ALTER TABLE catalog_entries ADD COLUMN resource_id TEXT"},
		{"content_hash", "ALTER TABLE catalog_entries ADD COLUMN content_hash TEXT"},
		{"identity_json", "ALTER TABLE catalog_entries ADD COLUMN identity_json TEXT"},
		{"version", "ALTER TABLE catalog_entries ADD COLUMN version INTEGER DEFAULT 1"},
	}

	for _, col := range columns {
		exists, err := c.columnExists("catalog_entries", col.name)
		if err != nil {
			return fmt.Errorf("failed to check column %s: %w", col.name, err)
		}
		if !exists {
			if _, err := c.db.Exec(col.ddl); err != nil {
				return fmt.Errorf("failed to add column %s: %w", col.name, err)
			}
		}
	}

	// Create indexes for identity queries
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_resource_id ON catalog_entries(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx_content_hash ON catalog_entries(content_hash)",
		"CREATE INDEX IF NOT EXISTS idx_resource_version ON catalog_entries(resource_id, version)",
	}
	for _, idx := range indexes {
		if _, err := c.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create identity index: %w", err)
		}
	}

	// Backfill defaults for existing rows
	if err := c.backfillDefaults(); err != nil {
		return fmt.Errorf("failed to backfill defaults: %w", err)
	}

	return nil
}

// columnExists checks if a column exists using PRAGMA table_info.
func (c *CatalogDB) columnExists(table, column string) (bool, error) {
	rows, err := c.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// backfillDefaults sets content_hash=id and version=1 for legacy rows.
// Only updates rows where content_hash is NULL (first run after upgrade).
func (c *CatalogDB) backfillDefaults() error {
	// Set content_hash = id for rows without content_hash
	_, err := c.db.Exec(`UPDATE catalog_entries SET content_hash = id WHERE content_hash IS NULL`)
	if err != nil {
		return fmt.Errorf("failed to backfill content_hash: %w", err)
	}

	// Set version = 1 for rows without version
	_, err = c.db.Exec(`UPDATE catalog_entries SET version = 1 WHERE version IS NULL`)
	if err != nil {
		return fmt.Errorf("failed to backfill version: %w", err)
	}

	return nil
}

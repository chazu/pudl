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

// ensureArtifactColumns adds artifact tracking columns if missing.
// Idempotent — safe to run on every DB open.
func (c *CatalogDB) ensureArtifactColumns() error {
	// Rename the legacy `definition` column to `target` on pre-existing DBs
	// (preserving data) before the add-column loop, so an old DB renames rather
	// than getting a fresh empty `target` beside the old `definition`.
	if err := c.renameLegacyDefinitionColumn(); err != nil {
		return err
	}

	columns := []struct {
		name string
		ddl  string
	}{
		{"entry_type", "ALTER TABLE catalog_entries ADD COLUMN entry_type TEXT DEFAULT 'import'"},
		{"target", "ALTER TABLE catalog_entries ADD COLUMN target TEXT"},
		{"run_id", "ALTER TABLE catalog_entries ADD COLUMN run_id TEXT"},
		{"tags", "ALTER TABLE catalog_entries ADD COLUMN tags TEXT"},
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

	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_entry_type ON catalog_entries(entry_type)",
		"CREATE INDEX IF NOT EXISTS idx_target ON catalog_entries(target)",
		"CREATE INDEX IF NOT EXISTS idx_run_id ON catalog_entries(run_id)",
	}
	for _, idx := range indexes {
		if _, err := c.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create artifact index: %w", err)
		}
	}

	// Backfill entry_type for existing rows
	if _, err := c.db.Exec(`UPDATE catalog_entries SET entry_type = 'import' WHERE entry_type IS NULL`); err != nil {
		return fmt.Errorf("failed to backfill entry_type: %w", err)
	}

	// Drop the legacy `method` column + its index. They are leftovers of the
	// removed definition→method→artifact model (the executor moved to mu); the
	// column has always been NULL. Drop the catalog_entry_edb view first since
	// it references the column (SQLite blocks DROP COLUMN otherwise);
	// ensureCatalogEntryView recreates the view later in the open sequence.
	if err := c.dropLegacyMethodColumn(); err != nil {
		return err
	}

	return nil
}

// dropLegacyMethodColumn removes the orphaned `method` column and its index
// from pre-existing databases. Idempotent — a no-op once the column is gone.
func (c *CatalogDB) dropLegacyMethodColumn() error {
	exists, err := c.columnExists("catalog_entries", "method")
	if err != nil {
		return fmt.Errorf("failed to check method column: %w", err)
	}
	if !exists {
		return nil
	}

	if _, err := c.db.Exec("DROP VIEW IF EXISTS " + CatalogEntryView); err != nil {
		return fmt.Errorf("failed to drop %s for method cleanup: %w", CatalogEntryView, err)
	}
	if _, err := c.db.Exec("DROP INDEX IF EXISTS idx_definition_method"); err != nil {
		return fmt.Errorf("failed to drop legacy method index: %w", err)
	}
	if _, err := c.db.Exec("ALTER TABLE catalog_entries DROP COLUMN method"); err != nil {
		return fmt.Errorf("failed to drop legacy method column: %w", err)
	}
	return nil
}

// renameLegacyDefinitionColumn renames the `definition` column to `target` on
// pre-existing databases, preserving data. "definition" was a fossil of the
// removed World-A subsystem; the column actually holds the mu target name that
// produced the rows, so `target` names it for what it is. Idempotent — a no-op
// once renamed (or on a fresh DB that never had `definition`). Drops the
// catalog_entry_edb view first since it references the column (SQLite blocks the
// rename otherwise); ensureCatalogEntryView recreates it later in the open
// sequence. Also retires the old idx_definition (the rename leaves the index on
// the renamed column but keeps its stale name).
func (c *CatalogDB) renameLegacyDefinitionColumn() error {
	hasOld, err := c.columnExists("catalog_entries", "definition")
	if err != nil {
		return fmt.Errorf("failed to check definition column: %w", err)
	}
	if !hasOld {
		return nil // already renamed, or fresh DB
	}
	hasNew, err := c.columnExists("catalog_entries", "target")
	if err != nil {
		return fmt.Errorf("failed to check target column: %w", err)
	}
	if hasNew {
		return nil // both present is unexpected; leave it for manual repair
	}

	if _, err := c.db.Exec("DROP VIEW IF EXISTS " + CatalogEntryView); err != nil {
		return fmt.Errorf("failed to drop %s for definition rename: %w", CatalogEntryView, err)
	}
	if _, err := c.db.Exec("DROP INDEX IF EXISTS idx_definition"); err != nil {
		return fmt.Errorf("failed to drop legacy definition index: %w", err)
	}
	if _, err := c.db.Exec("ALTER TABLE catalog_entries RENAME COLUMN definition TO target"); err != nil {
		return fmt.Errorf("failed to rename definition column to target: %w", err)
	}
	return nil
}

// ensureStatusColumn adds the convergence status column if missing.
// Idempotent — safe to run on every DB open.
func (c *CatalogDB) ensureStatusColumn() error {
	var count int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('catalog_entries') WHERE name='status'",
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // Already exists
	}

	_, err = c.db.Exec("ALTER TABLE catalog_entries ADD COLUMN status TEXT DEFAULT 'unknown'")
	return err
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

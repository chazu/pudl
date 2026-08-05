package database

import (
	"fmt"
	"time"
)

// This file is the catalog's schema history.
//
// Every schema step used to run on every open, kept safe by each author
// remembering to write its own guard — `CREATE TABLE IF NOT EXISTS`, a
// columnExists check, `CREATE INDEX IF NOT EXISTS`. That works until a step
// cannot be expressed idempotently, and it leaves no record of what a given
// database has actually had applied to it, so a half-migrated catalog is
// indistinguishable from a current one.
//
// Migrations are now an ordered list, applied in order and recorded on success.
//
// What is NOT here: views and syncs. `ensureCatalogEntryView` and
// `ensureFactScoredView` drop and recreate their views on every open so the
// definition always matches the Go source declaring it, and backfills are
// guarded by their own emptiness checks. A migration changes the schema's shape;
// a view or a sync restates it from the current source. Versioning a view would
// make a change to its body a no-op until someone bumped a number.

// migration is one ordered, recorded schema change.
type migration struct {
	version int
	name    string
	apply   func(*CatalogDB) error
}

// migrations is the ordered schema history. Append only; never renumber.
//
// Versions 1-8 describe the schema as it stood when this table was introduced.
// They are still written idempotently, which is what makes the transition safe:
// an existing database has no version rows, so on its next open every migration
// is unapplied and runs — exactly what happened before — and is recorded. From
// the second open onward they are skipped.
//
// Applied-ness is deliberately not inferred from the schema ("does this column
// exist?"). That marks a half-migrated database as current, which is the failure
// this table exists to prevent.
var migrations = []migration{
	{1, "catalog_entries", (*CatalogDB).migrateCatalogEntries},
	{2, "identity_columns", (*CatalogDB).ensureIdentityColumns},
	{3, "artifact_columns", (*CatalogDB).ensureArtifactColumns},
	{4, "status_column", (*CatalogDB).ensureStatusColumn},
	{5, "collection_memberships", (*CatalogDB).ensureCollectionMembershipsTable},
	{6, "runs", (*CatalogDB).ensureRunsTable},
	{7, "observe_snapshots", (*CatalogDB).ensureObserveSnapshotsTable},
	{8, "facts", (*CatalogDB).ensureFactsTable},
	{9, "current_facts", (*CatalogDB).ensureCurrentFactsTable},
	{10, "facts_fts", (*CatalogDB).ensureFactsFTSTable},
	{11, "item_schemas", (*CatalogDB).ensureItemSchemasTable},
	{12, "retire_legacy_collection_columns", (*CatalogDB).retireLegacyCollectionColumns},
	{13, "run_reports", (*CatalogDB).ensureRunReportsTable},
	{14, "run_approvals", (*CatalogDB).ensureRunApprovalsTable},
	{15, "run_set_reports", (*CatalogDB).ensureRunSetReportsTable},
	{16, "run_set_approvals", (*CatalogDB).ensureRunSetApprovalsTable},
}

// ensureMigrationsTable creates the version ledger itself. It is the one step
// that cannot be versioned, so it stays a plain idempotent create.
func (c *CatalogDB) ensureMigrationsTable() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

// appliedMigrations reads the versions this database has recorded.
func (c *CatalogDB) appliedMigrations() (map[int]bool, error) {
	rows, err := c.db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// runMigrations applies every unrecorded migration in order.
//
// A migration is recorded only after it returns without error, so a failure
// leaves it unrecorded and it is retried on the next open. Nothing continues
// past a failure: a later migration may assume an earlier one's shape.
func (c *CatalogDB) runMigrations() error {
	if err := c.ensureMigrationsTable(); err != nil {
		return err
	}
	applied, err := c.appliedMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := m.apply(c); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := c.db.Exec(
			`INSERT OR REPLACE INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("record migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

// AppliedMigrationVersions reports which schema versions this catalog has
// recorded, for diagnostics.
func (c *CatalogDB) AppliedMigrationVersions() ([]int, error) {
	applied, err := c.appliedMigrations()
	if err != nil {
		return nil, err
	}
	versions := make([]int, 0, len(applied))
	for _, m := range migrations {
		if applied[m.version] {
			versions = append(versions, m.version)
		}
	}
	return versions, nil
}

// migrateCatalogEntries creates the base table and its indexes.
//
// The collection_id / item_index columns are absent from this DDL: migration 12
// retires them in favour of collection_memberships, and a fresh database has no
// reason to create what the next migration drops. Databases that predate it
// still have the columns and are handled there.
func (c *CatalogDB) migrateCatalogEntries() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS catalog_entries (
		id TEXT PRIMARY KEY,
		stored_path TEXT NOT NULL,
		metadata_path TEXT NOT NULL,
		import_timestamp DATETIME NOT NULL,
		format TEXT NOT NULL,
		origin TEXT NOT NULL,
		schema TEXT NOT NULL,
		confidence REAL NOT NULL,
		record_count INTEGER NOT NULL,
		size_bytes INTEGER NOT NULL,
		collection_type TEXT,         -- 'collection', 'item', or NULL for standalone
		item_id TEXT,                 -- Unique identifier for collection items
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := c.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create catalog_entries table: %w", err)
	}

	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_collection_type ON catalog_entries(collection_type);",
		"CREATE INDEX IF NOT EXISTS idx_item_id ON catalog_entries(item_id);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_schema ON catalog_entries(schema);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_origin ON catalog_entries(origin);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_format ON catalog_entries(format);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_import_timestamp ON catalog_entries(import_timestamp);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_size_bytes ON catalog_entries(size_bytes);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_record_count ON catalog_entries(record_count);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_confidence ON catalog_entries(confidence);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_created_at ON catalog_entries(created_at);",
	}
	for _, sql := range indexes {
		if _, err := c.db.Exec(sql); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}
	return nil
}

// retireLegacyCollectionColumns drops catalog_entries.collection_id and
// item_index, leaving collection_memberships as the sole relationship source.
//
// The columns were written once, at insert, and three paths have since made them
// wrong: dedup reuses a record entry and adds a membership (so a record in three
// snapshots names only the first), removal drops memberships without touching the
// column, and pruning deletes a snapshot while any surviving shared record keeps
// a collection_id pointing at a collection that no longer exists. This is not a
// tidy-up — the column is a second answer to a question the membership table
// already answers correctly, and it is already the wrong one.
//
// Ordering: migration 5 backfills memberships *from* these columns, so it must
// have run first. The migration list guarantees that, and the backfill is itself
// conditional on the columns existing so it is correct on a database where they
// are already gone.
//
// One-way. An older pudl binary would select a column that no longer exists.
// That is what retiring a column means; the membership data is strictly richer.
func (c *CatalogDB) retireLegacyCollectionColumns() error {
	// The catalog_entry_edb view selects collection_id, and SQLite refuses to drop
	// a column any view references. Dropping it here is safe and not a special
	// case: createTables recreates every view after the migrations precisely
	// because a view is a restatement of the current source.
	if _, err := c.db.Exec("DROP VIEW IF EXISTS " + CatalogEntryView); err != nil {
		return fmt.Errorf("drop view %s: %w", CatalogEntryView, err)
	}

	// The indexes reference the columns; SQLite refuses to drop a column an index
	// depends on.
	for _, index := range []string{"idx_collection_id", "idx_item_index"} {
		if _, err := c.db.Exec("DROP INDEX IF EXISTS " + index); err != nil {
			return fmt.Errorf("drop index %s: %w", index, err)
		}
	}

	for _, column := range []string{"collection_id", "item_index"} {
		exists, err := c.columnExists("catalog_entries", column)
		if err != nil {
			return fmt.Errorf("check column %s: %w", column, err)
		}
		if !exists {
			continue
		}
		if _, err := c.db.Exec("ALTER TABLE catalog_entries DROP COLUMN " + column); err != nil {
			return fmt.Errorf("drop column %s: %w", column, err)
		}
	}
	return nil
}

package database

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
)

// catalogEntryViewMetaKey records, in catalog_meta, the hash of the view
// definition the database currently holds.
const catalogEntryViewMetaKey = "view:" + CatalogEntryView

// catalogEntryViewSQL is the catalog_entry_edb definition. It exposes a
// curated, stable subset of catalog_entries columns as the Datalog
// catalog_entry relation. Internal/volatile columns (stored_path,
// metadata_path, identity_json, tags, timestamps, byte/record counts) are
// excluded so the datalog interface does not depend on physical storage
// details.
//
// collection_id is derived from collection_memberships rather than read from a
// column, because the column was retired: an item can belong to several
// collections, so the stored value named whichever inserted it first and went
// stale as soon as the item was shared, a membership was removed, or a snapshot
// was pruned. A rule joining on collection_id now sees the unambiguous
// membership, or nothing when there is more than one.
var catalogEntryViewSQL = fmt.Sprintf(`CREATE VIEW %s AS
		SELECT
			id,
			schema,
			origin,
			format,
			status,
			entry_type,
			target,
			run_id,
			resource_id,
			content_hash,
			version,
			(SELECT m.collection_id FROM collection_memberships m
			   WHERE m.item_id = catalog_entries.id
			   GROUP BY m.item_id HAVING COUNT(*) = 1) AS collection_id,
			collection_type,
			item_id
		FROM catalog_entries;`, CatalogEntryView)

// ensureCatalogEntryView makes the database's catalog_entry_edb view match
// catalogEntryViewSQL.
//
// Must run after all catalog_entries column migrations, since the view
// references migration-added columns (status, entry_type, target, run_id,
// resource_id, content_hash, version).
//
// The view is rebuilt only when its definition hash differs from the one
// recorded in catalog_meta, or when the view is missing (column migrations drop
// it). An unchanged open therefore performs no schema write: dropping and
// recreating it on every open took the write lock, changed the schema version
// for every other connection, and left a window in which a concurrent reader's
// catalog join failed with "no such table". A rebuild drops, recreates and
// records the hash in one transaction, so readers see either definition, never
// neither. Editing catalogEntryViewSQL changes the hash, so a new definition
// takes effect on the next open without anyone bumping a number.
func (c *CatalogDB) ensureCatalogEntryView() error {
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(catalogEntryViewSQL)))

	var have string
	err := c.db.QueryRow(`SELECT value FROM catalog_meta WHERE key = ?`, catalogEntryViewMetaKey).Scan(&have)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read %s definition hash: %w", CatalogEntryView, err)
	}
	var views int
	if err := c.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'view' AND name = ?`, CatalogEntryView).Scan(&views); err != nil {
		return fmt.Errorf("check view %s: %w", CatalogEntryView, err)
	}
	if have == want && views == 1 {
		return nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DROP VIEW IF EXISTS " + CatalogEntryView); err != nil {
		return fmt.Errorf("drop view %s: %w", CatalogEntryView, err)
	}
	if _, err := tx.Exec(catalogEntryViewSQL); err != nil {
		return fmt.Errorf("create view %s: %w", CatalogEntryView, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO catalog_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, catalogEntryViewMetaKey, want); err != nil {
		return fmt.Errorf("record %s definition hash: %w", CatalogEntryView, err)
	}
	return tx.Commit()
}

// ensureCatalogMetaTable creates the key/value table that records derived
// schema state, such as which view definitions a database holds.
func (c *CatalogDB) ensureCatalogMetaTable() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS catalog_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`)
	return err
}

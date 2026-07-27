package database

import "fmt"

// ensureCatalogEntryView creates the catalog_entry_edb view that exposes a
// curated, stable subset of catalog_entries columns as the Datalog
// catalog_entry relation. Internal/volatile columns (stored_path,
// metadata_path, identity_json, tags, timestamps, byte/record counts) are
// excluded so the datalog interface does not depend on physical storage
// details.
//
// Must run after all catalog_entries column migrations, since the view
// references migration-added columns (status, entry_type, target,
// run_id, resource_id, content_hash, version). It is dropped and recreated on
// every open so the view definition always matches this source.
//
// collection_id is derived from collection_memberships rather than read from a
// column, because the column was retired: an item can belong to several
// collections, so the stored value named whichever inserted it first and went
// stale as soon as the item was shared, a membership was removed, or a snapshot
// was pruned. A rule joining on collection_id now sees the unambiguous
// membership, or nothing when there is more than one.
func (c *CatalogDB) ensureCatalogEntryView() error {
	if _, err := c.db.Exec("DROP VIEW IF EXISTS " + CatalogEntryView); err != nil {
		return fmt.Errorf("drop view %s: %w", CatalogEntryView, err)
	}

	createView := fmt.Sprintf(`CREATE VIEW %s AS
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

	if _, err := c.db.Exec(createView); err != nil {
		return fmt.Errorf("create view %s: %w", CatalogEntryView, err)
	}
	return nil
}

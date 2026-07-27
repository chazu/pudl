package database

import "fmt"

// ensureCollectionMembershipsTable creates the normalized collection/item
// relationship. The legacy collection_id columns remain for compatibility,
// but membership is now the source of truth for collection contents.
func (c *CatalogDB) ensureCollectionMembershipsTable() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS collection_memberships (
			collection_id TEXT NOT NULL,
			item_id TEXT NOT NULL,
			item_index INTEGER NOT NULL,
			PRIMARY KEY (collection_id, item_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_collection_memberships_item ON collection_memberships(item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_collection_memberships_collection ON collection_memberships(collection_id, item_index)`,
	}
	for _, statement := range statements {
		if _, err := c.db.Exec(statement); err != nil {
			return fmt.Errorf("collection membership migration: %w", err)
		}
	}

	// Backfill from the legacy columns, on a database old enough to still have
	// them. Conditional because the next migration drops them: this has to be
	// correct whether it runs before or after, on any database.
	legacy, err := c.columnExists("catalog_entries", "collection_id")
	if err != nil {
		return fmt.Errorf("collection membership migration: %w", err)
	}
	if !legacy {
		return nil
	}
	if _, err := c.db.Exec(
		`INSERT OR IGNORE INTO collection_memberships (collection_id, item_id, item_index)
		 SELECT collection_id, id, COALESCE(item_index, 0)
		 FROM catalog_entries
		 WHERE collection_type = 'item' AND collection_id IS NOT NULL`); err != nil {
		return fmt.Errorf("collection membership backfill: %w", err)
	}
	return nil
}

// AddCollectionMembership associates an existing content-addressed item with
// a collection. Re-adding an item updates its position deterministically.
func (c *CatalogDB) AddCollectionMembership(collectionID, itemID string, itemIndex int) error {
	return addCollectionMembershipIn(c.db, collectionID, itemID, itemIndex)
}

// addCollectionMembershipIn is the executor-parameterized form, so a membership
// can be written inside the same transaction as the entry it belongs to.
func addCollectionMembershipIn(q dbtx, collectionID, itemID string, itemIndex int) error {
	_, err := q.Exec(`
		INSERT INTO collection_memberships (collection_id, item_id, item_index)
		VALUES (?, ?, ?)
		ON CONFLICT(collection_id, item_id) DO UPDATE SET item_index = excluded.item_index`,
		collectionID, itemID, itemIndex)
	if err != nil {
		return fmt.Errorf("add collection membership: %w", err)
	}
	return nil
}

// RemoveCollectionMembership removes one collection/item relationship.
func (c *CatalogDB) RemoveCollectionMembership(collectionID, itemID string) error {
	if _, err := c.db.Exec("DELETE FROM collection_memberships WHERE collection_id = ? AND item_id = ?", collectionID, itemID); err != nil {
		return fmt.Errorf("remove collection membership: %w", err)
	}
	return nil
}

// ItemMembershipCount reports how many collections still retain an item.
func (c *CatalogDB) ItemMembershipCount(itemID string) (int, error) {
	var count int
	if err := c.db.QueryRow("SELECT COUNT(*) FROM collection_memberships WHERE item_id = ?", itemID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count item memberships: %w", err)
	}
	return count, nil
}

// RemoveCollectionMemberships removes all relationships owned by a collection.
func (c *CatalogDB) RemoveCollectionMemberships(collectionID string) error {
	if _, err := c.db.Exec("DELETE FROM collection_memberships WHERE collection_id = ?", collectionID); err != nil {
		return fmt.Errorf("remove collection memberships: %w", err)
	}
	return nil
}

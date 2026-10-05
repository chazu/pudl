package database

// Restored observations remain historical evidence, but cannot authorize a
// producer binding until a new live observation replaces them. Keep this
// separate from runs so restoring does not rewrite historical run conclusions.
func (c *CatalogDB) ensureSnapshotReuseBlocks() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS snapshot_reuse_blocks (snapshot_id TEXT PRIMARY KEY, reason TEXT NOT NULL)`)
	return err
}

func (c *CatalogDB) snapshotReuseCondition(alias string) (string, error) {
	var exists bool
	if err := c.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='snapshot_reuse_blocks')`).Scan(&exists); err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	return " AND NOT EXISTS(SELECT 1 FROM snapshot_reuse_blocks b WHERE b.snapshot_id=" + alias + ".snapshot_id)", nil
}

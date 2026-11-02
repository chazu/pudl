package database

// Restored observations remain historical evidence, but retention does not
// treat them as a model's latest successful observation until a new live
// observation replaces them. Keep this separate from runs so restoring does not
// rewrite historical run conclusions.
func (c *CatalogDB) ensureSnapshotReuseBlocks() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS snapshot_reuse_blocks (snapshot_id TEXT PRIMARY KEY, reason TEXT NOT NULL)`)
	return err
}

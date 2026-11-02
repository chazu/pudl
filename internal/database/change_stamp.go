package database

import "fmt"

// ChangeStamp is a cheap fingerprint of the catalog's state: it changes
// whenever an entry is added, removed or updated, a fact is written, or a
// snapshot is recorded or pruned. It is opaque; compare stamps for equality.
// Long-running readers (pudl ui) poll it to know when to re-read.
func (c *CatalogDB) ChangeStamp() (string, error) {
	var factSeq, entries, maxRow, snapshots int64
	var lastUpdate, lastSnapshot string
	err := c.db.QueryRow(`SELECT
		(SELECT COALESCE(MAX(next), 0) FROM fact_sequence),
		(SELECT COUNT(*) FROM catalog_entries),
		(SELECT COALESCE(MAX(rowid), 0) FROM catalog_entries),
		(SELECT COALESCE(CAST(MAX(updated_at) AS TEXT), '') FROM catalog_entries),
		(SELECT COUNT(*) FROM observe_snapshots),
		(SELECT COALESCE(CAST(MAX(created_at) AS TEXT), '') FROM observe_snapshots)`).
		Scan(&factSeq, &entries, &maxRow, &lastUpdate, &snapshots, &lastSnapshot)
	if err != nil {
		return "", fmt.Errorf("catalog change stamp: %w", err)
	}
	return fmt.Sprintf("f%d.e%d.r%d.u%s.s%d.t%s", factSeq, entries, maxRow, lastUpdate, snapshots, lastSnapshot), nil
}

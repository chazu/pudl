package database

import (
	"context"
	"time"
)

type ResourceObservation struct {
	EntryID    string    `json:"entry_id"`
	SnapshotID string    `json:"snapshot_id"`
	ObservedAt time.Time `json:"observed_at"`
	Scope      string    `json:"scope"`
	Source     string    `json:"source"`
	Complete   bool      `json:"complete"`
}

// ResourceObservations preserves re-observation chronology even when unchanged
// bytes deduplicate to an older content entry.
func (c *CatalogDB) ResourceObservations(ctx context.Context, resourceID string) ([]ResourceObservation, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT DISTINCT e.id,s.snapshot_id,s.created_at,s.scope,s.source,s.complete
 FROM catalog_entries e JOIN collection_memberships m ON m.item_id=e.id
 JOIN observe_snapshots s ON s.snapshot_id=m.collection_id
 WHERE e.resource_id=? ORDER BY s.created_at DESC,s.snapshot_id`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResourceObservation{}
	for rows.Next() {
		var r ResourceObservation
		if err := rows.Scan(&r.EntryID, &r.SnapshotID, &r.ObservedAt, &r.Scope, &r.Source, &r.Complete); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

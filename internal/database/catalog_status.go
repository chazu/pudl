package database

import (
	"fmt"
	"time"
)

// TargetStatus represents the convergence status of a single target.
type TargetStatus struct {
	Target    string
	Status    string
	UpdatedAt time.Time
	DiffCount int // from latest drift report, 0 if clean
}

// UpdateStatus sets the convergence status for entries matching a target name.
// Only updates the latest entry for the target.
func (c *CatalogDB) UpdateStatus(targetName string, status string) error {
	return updateStatusIn(c.db, targetName, status)
}

// updateStatusIn is the executor-parameterized form of UpdateStatus, so the
// per-action statuses of one apply commit with the manifest that produced them.
func updateStatusIn(q dbtx, targetName string, status string) error {
	validStatuses := map[string]bool{
		"unknown": true, "clean": true, "drifted": true,
		"converging": true, "failed": true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	_, err := q.Exec(
		`UPDATE catalog_entries SET status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE target = ? AND id = (
		     SELECT id FROM catalog_entries WHERE target = ?
		     ORDER BY import_timestamp DESC LIMIT 1
		 )`,
		status, targetName, targetName,
	)
	return err
}

// GetTargetStatuses returns the latest status for each target that has entries.
func (c *CatalogDB) GetTargetStatuses() ([]TargetStatus, error) {
	return getTargetStatusesIn(c.db)
}

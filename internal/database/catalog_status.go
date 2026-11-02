package database

import (
	"fmt"
	"time"
)

// legacyConvergingStatus was written by the retired convergence workflow for
// an applied but unverified change. Rows that still hold it read as unknown,
// so a missing observation is never shown as clean; the stored value is left
// as it was recorded.
const legacyConvergingStatus = "converging"

// readStatus maps a stored status to the vocabulary readers see.
func readStatus(status string) string {
	if status == legacyConvergingStatus {
		return "unknown"
	}
	return status
}

// TargetStatus represents the verification status of a single target.
type TargetStatus struct {
	Target    string
	Status    string
	UpdatedAt time.Time
	DiffCount int // from latest drift report, 0 if clean
}

// UpdateStatus sets the verification status for entries matching a target name.
// Only updates the latest entry for the target.
func (c *CatalogDB) UpdateStatus(targetName string, status string) error {
	return updateStatusIn(c.db, targetName, status)
}

// updateStatusIn is the executor-parameterized form of UpdateStatus, so a
// status can commit with the observation that produced it.
func updateStatusIn(q dbtx, targetName string, status string) error {
	validStatuses := map[string]bool{
		"unknown": true, "clean": true, "drifted": true, "failed": true,
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

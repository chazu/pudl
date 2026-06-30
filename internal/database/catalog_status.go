package database

import (
	"fmt"
	"time"
)

// TargetStatus represents the convergence status of a single target.
type TargetStatus struct {
	Target string
	Status     string
	UpdatedAt  time.Time
	DiffCount  int // from latest drift report, 0 if clean
}

// UpdateStatus sets the convergence status for entries matching a target name.
// Only updates the latest entry for the target.
func (c *CatalogDB) UpdateStatus(targetName string, status string) error {
	validStatuses := map[string]bool{
		"unknown": true, "clean": true, "drifted": true,
		"converging": true, "failed": true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	_, err := c.db.Exec(
		`UPDATE catalog_entries SET status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE target = ? AND id = (
		     SELECT id FROM catalog_entries WHERE target = ?
		     ORDER BY import_timestamp DESC LIMIT 1
		 )`,
		status, targetName, targetName,
	)
	return err
}

// PromoteConvergingToClean flips status converging -> clean for the latest entry
// of each named target that is currently "converging", returning the number
// promoted. It is the drift re-check verifying a pending apply: when a model's
// drift is ∅, its resources that ingest-manifest left "applied, pending
// verification" (converging) are now confirmed in sync. Targets not currently
// converging (or absent) are untouched, so it is safe to call with a superset of
// candidate names.
func (c *CatalogDB) PromoteConvergingToClean(targets []string) (int, error) {
	promoted := 0
	for _, def := range targets {
		res, err := c.db.Exec(
			`UPDATE catalog_entries SET status = 'clean', updated_at = CURRENT_TIMESTAMP
			 WHERE target = ? AND status = 'converging' AND id = (
			     SELECT id FROM catalog_entries WHERE target = ?
			     ORDER BY import_timestamp DESC LIMIT 1
			 )`,
			def, def,
		)
		if err != nil {
			return promoted, fmt.Errorf("promote %q: %w", def, err)
		}
		n, _ := res.RowsAffected()
		promoted += int(n)
	}
	return promoted, nil
}

// PromoteConvergingToCleanByModel flips status converging -> clean for every entry
// tagged with the given model (`tags.model`, set by `ingest-manifest --model`). This
// is the exact form of the drift re-check verifying a pending apply: when the model's
// drift is ∅, all its resources that ingest-manifest left "converging" are confirmed
// in sync — without reconstructing the resource→model mapping from desired records.
// Returns the number promoted.
func (c *CatalogDB) PromoteConvergingToCleanByModel(model string) (int, error) {
	if model == "" {
		return 0, nil
	}
	res, err := c.db.Exec(
		`UPDATE catalog_entries SET status = 'clean', updated_at = CURRENT_TIMESTAMP
		 WHERE status = 'converging' AND json_extract(tags, '$.model') = ?`,
		model,
	)
	if err != nil {
		return 0, fmt.Errorf("promote converging for model %q: %w", model, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// GetTargetStatuses returns the latest status for each target that has entries.
func (c *CatalogDB) GetTargetStatuses() ([]TargetStatus, error) {
	rows, err := c.db.Query(`
		SELECT target, status, updated_at
		FROM catalog_entries
		WHERE target IS NOT NULL AND target != ''
		GROUP BY target
		HAVING import_timestamp = MAX(import_timestamp)
		ORDER BY target`)
	if err != nil {
		return nil, fmt.Errorf("failed to query target statuses: %w", err)
	}
	defer rows.Close()

	var statuses []TargetStatus
	for rows.Next() {
		var ds TargetStatus
		var statusVal *string
		if err := rows.Scan(&ds.Target, &statusVal, &ds.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan target status: %w", err)
		}
		if statusVal != nil {
			ds.Status = *statusVal
		} else {
			ds.Status = "unknown"
		}
		statuses = append(statuses, ds)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating target statuses: %w", err)
	}

	return statuses, nil
}

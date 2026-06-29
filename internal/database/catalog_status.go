package database

import (
	"fmt"
	"time"
)

// DefinitionStatus represents the convergence status of a single definition.
type DefinitionStatus struct {
	Definition string
	Status     string
	UpdatedAt  time.Time
	DiffCount  int // from latest drift report, 0 if clean
}

// UpdateStatus sets the convergence status for entries matching a definition name.
// Only updates the latest entry for the definition.
func (c *CatalogDB) UpdateStatus(definitionName string, status string) error {
	validStatuses := map[string]bool{
		"unknown": true, "clean": true, "drifted": true,
		"converging": true, "failed": true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	_, err := c.db.Exec(
		`UPDATE catalog_entries SET status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE definition = ? AND id = (
		     SELECT id FROM catalog_entries WHERE definition = ?
		     ORDER BY import_timestamp DESC LIMIT 1
		 )`,
		status, definitionName, definitionName,
	)
	return err
}

// PromoteConvergingToClean flips status converging -> clean for the latest entry
// of each named definition that is currently "converging", returning the number
// promoted. It is the drift re-check verifying a pending apply: when a model's
// drift is ∅, its resources that ingest-manifest left "applied, pending
// verification" (converging) are now confirmed in sync. Definitions not currently
// converging (or absent) are untouched, so it is safe to call with a superset of
// candidate names.
func (c *CatalogDB) PromoteConvergingToClean(definitions []string) (int, error) {
	promoted := 0
	for _, def := range definitions {
		res, err := c.db.Exec(
			`UPDATE catalog_entries SET status = 'clean', updated_at = CURRENT_TIMESTAMP
			 WHERE definition = ? AND status = 'converging' AND id = (
			     SELECT id FROM catalog_entries WHERE definition = ?
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

// GetDefinitionStatuses returns the latest status for each definition that has entries.
func (c *CatalogDB) GetDefinitionStatuses() ([]DefinitionStatus, error) {
	rows, err := c.db.Query(`
		SELECT definition, status, updated_at
		FROM catalog_entries
		WHERE definition IS NOT NULL AND definition != ''
		GROUP BY definition
		HAVING import_timestamp = MAX(import_timestamp)
		ORDER BY definition`)
	if err != nil {
		return nil, fmt.Errorf("failed to query definition statuses: %w", err)
	}
	defer rows.Close()

	var statuses []DefinitionStatus
	for rows.Next() {
		var ds DefinitionStatus
		var statusVal *string
		if err := rows.Scan(&ds.Definition, &statusVal, &ds.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan definition status: %w", err)
		}
		if statusVal != nil {
			ds.Status = *statusVal
		} else {
			ds.Status = "unknown"
		}
		statuses = append(statuses, ds)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating definition statuses: %w", err)
	}

	return statuses, nil
}

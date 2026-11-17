package database

import (
	"fmt"
	"strings"
)

// ensureObservationInput adds the enumerated input of a per-input
// observation to its snapshot, and the scope to tombstones, so the previous
// observation of a scope is found even when it was pruned. Idempotent.
func (c *CatalogDB) ensureObservationInput() error {
	for _, col := range []struct{ table, name string }{{"observe_snapshots", "input"}, {"observe_snapshot_tombstones", "scope"}} {
		exists, err := c.columnExists(col.table, col.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := c.db.Exec("ALTER TABLE " + col.table + " ADD COLUMN " + col.name + " TEXT NOT NULL DEFAULT ''"); err != nil {
				return fmt.Errorf("observation input migration: %w", err)
			}
		}
	}
	return nil
}

// ModelObservations returns the model's latest observation: its newest
// snapshot, or, when that snapshot is per input, every per-input snapshot
// the same run recorded, in input order. It is empty when the model has
// none.
func (c *CatalogDB) ModelObservations(model string) ([]ObserveSnapshot, error) {
	newest, err := c.CurrentObserveSnapshot(model)
	if err != nil || newest == nil {
		return nil, err
	}
	if newest.Input == "" || newest.RunID == "" {
		return []ObserveSnapshot{*newest}, nil
	}
	args := []any{model, newest.RunID, newest.Workspace}
	placeholders := make([]string, len(observationSources))
	for i, source := range observationSources {
		placeholders[i] = "?"
		args = append(args, source)
	}
	rows, err := c.db.Query(
		`SELECT `+c.snapshotSelectColumns("observe_snapshots")+` FROM observe_snapshots
		 WHERE model = ? AND run_id = ? AND workspace = ? AND input != ''
		 AND source IN (`+strings.Join(placeholders, ", ")+`)
		 ORDER BY rowid`, args...)
	if err != nil {
		return nil, fmt.Errorf("observations of %q: %w", model, err)
	}
	defer rows.Close()
	var out []ObserveSnapshot
	for rows.Next() {
		snapshot, err := scanObserveSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan observation of %q: %w", model, err)
		}
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

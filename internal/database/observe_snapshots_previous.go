package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// PreviousSuccessfulObserveSnapshot selects one earlier eligible observation in
// the exact model/workspace. Pruned metadata wins over an older surviving row:
// silently skipping it would misrepresent which observation was previous.
func (c *CatalogDB) PreviousSuccessfulObserveSnapshot(ctx context.Context, currentID string) (*ObserveSnapshot, bool, error) {
	current, err := c.GetObserveSnapshot(currentID)
	if err != nil || current == nil {
		return nil, false, err
	}
	if current.Model == "" || current.Workspace == "" {
		return nil, false, nil
	}

	sources := make([]string, len(observationSources))
	args := []any{currentID, current.Model, current.Workspace, RunStatusSucceeded}
	for i, source := range observationSources {
		sources[i] = "?"
		args = append(args, source)
	}
	candidates := `SELECT snapshot_id,run_id,model,workspace,created_at,source,rowid AS snapshot_order,0 AS pruned FROM observe_snapshots`
	var hasTombstones bool
	if err := c.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='observe_snapshot_tombstones')`).Scan(&hasTombstones); err != nil {
		return nil, false, err
	}
	if hasTombstones {
		candidates += ` UNION ALL SELECT snapshot_id,run_id,model,workspace,created_at,source,snapshot_order,1 AS pruned FROM observe_snapshot_tombstones`
	}
	query := `WITH candidates AS (` + candidates + `) SELECT s.snapshot_id,s.run_id,s.model,s.workspace,s.created_at,s.source,s.pruned
 FROM candidates s JOIN runs r ON r.run_id=s.run_id AND r.model=s.model
 JOIN observe_snapshots current ON current.snapshot_id=?
 WHERE s.model=? AND s.workspace=? AND r.completion_status=?
 AND (s.created_at<current.created_at OR (s.created_at=current.created_at AND s.snapshot_order<current.rowid))
 AND s.source IN (` + strings.Join(sources, ",") + `)
 ORDER BY s.created_at DESC,s.snapshot_order DESC LIMIT 1`
	var snapshot ObserveSnapshot
	var pruned bool
	err = c.db.QueryRowContext(ctx, query, args...).Scan(&snapshot.SnapshotID, &snapshot.RunID, &snapshot.Model, &snapshot.Workspace, &snapshot.CreatedAt, &snapshot.Source, &pruned)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("previous observation for %q: %w", currentID, err)
	}
	return &snapshot, pruned, nil
}

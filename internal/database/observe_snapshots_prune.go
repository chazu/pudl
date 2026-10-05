package database

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/artifacts"
)

// PruneOptions selects which snapshots a prune may remove.
//
// Keep and OlderThan are an AND, deliberately. A prune that empties a model's
// history because a flag defaulted to zero is a more expensive surprise than one
// that does nothing; the explicit form (Keep 0 with an OlderThan in the future)
// is still available to anyone who means it.
type PruneOptions struct {
	// Model restricts the prune to one model. Empty means every model, still
	// applying Keep per model rather than globally.
	Model string
	// Keep is how many of the newest snapshots each model always retains.
	Keep int
	// OlderThan drops only snapshots created before this instant. Zero means no
	// age condition, in which case Keep alone decides.
	OlderThan time.Time
	// DryRun reports what would be removed and removes nothing.
	DryRun bool
	// DataDir bounds raw-file deletion: only files under <DataDir>/raw are
	// unlinked. Empty disables file removal entirely.
	DataDir string
}

// PruneResult reports what a prune did, or would do.
type PruneResult struct {
	Snapshots []string
	// Records is the number of observe item entries deleted — those left in no
	// remaining snapshot. Items shared with a surviving snapshot are not counted
	// because they are not removed.
	Records int
	// FilesRemoved counts raw files unlinked.
	FilesRemoved int
	// FilesSkipped lists stored paths left alone because they lie outside the
	// data dir's raw/ tree. Reported rather than silently ignored: an unexpected
	// path means an entry was staged somewhere this code does not understand.
	FilesSkipped []string
}

// PruneObserveSnapshots removes snapshots a retention policy no longer wants,
// with their memberships, their collection entries, and any observe record left
// in no snapshot at all.
//
// Records are content-addressed and deliberately shared between snapshots — that
// is what the ingest's dedup does — so what decides a record's fate is its
// remaining membership count, not which snapshot is being pruned. Deleting a
// pruned snapshot's records outright would silently empty whichever other
// snapshots still cited them.
//
// Snapshots with no observe_snapshots row are never pruned: they carry no model
// and no retention flag, so there is no policy to evaluate against them, and
// deleting what cannot be evaluated is not a policy.
func (c *CatalogDB) PruneObserveSnapshots(opts PruneOptions) (PruneResult, error) {
	return c.PruneObserveSnapshotsContext(context.Background(), opts)
}

func (c *CatalogDB) PruneObserveSnapshotsContext(ctx context.Context, opts PruneOptions) (PruneResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result PruneResult
	err := artifacts.WithLock(ctx, c.configDir, func() error {
		if opts.DryRun {
			victims, err := c.selectPruneVictims(opts)
			if err != nil {
				return err
			}
			result.Snapshots = victims
			records, paths, err := c.pruneImpact(victims)
			if err != nil {
				return err
			}
			result.Records = records
			for _, path := range paths {
				if prunableRawFile(path, opts.DataDir) {
					result.FilesRemoved++
				} else if path != "" {
					result.FilesSkipped = append(result.FilesSkipped, path)
				}
			}
			return nil
		}
		var orphanPaths []string
		err := c.WithCatalogTxContext(ctx, func(tx *CatalogTx) error {
			// Selection occurs under the same write lock as deletion, so a pin which
			// commits before this step can never be missed by a stale victim list.
			victims, err := selectPruneVictimsIn(tx.q, opts)
			if err != nil {
				return err
			}
			result.Snapshots = victims
			for _, id := range victims {
				paths, err := deleteSnapshotIn(tx.q, id)
				if err != nil {
					return err
				}
				orphanPaths = append(orphanPaths, paths...)
			}
			return nil
		})
		if err != nil {
			return err
		}
		result.Records = len(orphanPaths)
		for _, path := range orphanPaths {
			if !prunableRawFile(path, opts.DataDir) {
				if path != "" {
					result.FilesSkipped = append(result.FilesSkipped, path)
				}
				continue
			}
			removed, err := c.removeCommittedOrphanAt(path, opts.DataDir)
			if err != nil {
				result.FilesSkipped = append(result.FilesSkipped, path)
			} else if removed {
				result.FilesRemoved++
			}
		}
		return nil
	})
	return result, err
}

// selectPruneVictims lists the snapshots the policy removes, newest-first per
// model so the Keep window is the newest N.
func (c *CatalogDB) selectPruneVictims(opts PruneOptions) ([]string, error) {
	return selectPruneVictimsIn(c.db, opts)
}

func selectPruneVictimsIn(q dbtx, opts PruneOptions) ([]string, error) {
	query := `SELECT ` + observeSnapshotColumns + ` FROM observe_snapshots`
	var args []any
	if opts.Model != "" {
		query += ` WHERE model=?`
		args = append(args, opts.Model)
	}
	query += ` ORDER BY created_at DESC,rowid DESC`
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []ObserveSnapshot
	for rows.Next() {
		snapshot, err := scanObserveSnapshot(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	seenPerModel := map[string]int{}
	latestSuccessful := map[string]bool{}
	var victims []string
	for _, snapshot := range snapshots {
		position := seenPerModel[snapshot.Model]
		seenPerModel[snapshot.Model]++
		var pinned int
		if err := q.QueryRow(`SELECT COUNT(*) FROM snapshot_pins WHERE snapshot_id=? AND (expires_at IS NULL OR julianday(expires_at)>julianday(?))`, snapshot.SnapshotID, formatCatalogTime(time.Now().UTC())).Scan(&pinned); err != nil {
			return nil, err
		}
		eligible := false
		for _, source := range observationSources {
			if source == snapshot.Source {
				var count int
				if err := q.QueryRow(`SELECT COUNT(*) FROM runs WHERE run_id=? AND model=? AND completion_status=? AND NOT EXISTS(SELECT 1 FROM snapshot_reuse_blocks b WHERE b.snapshot_id=?)`, snapshot.RunID, snapshot.Model, RunStatusSucceeded, snapshot.SnapshotID).Scan(&count); err != nil {
					return nil, err
				}
				eligible = count > 0
				break
			}
		}
		scope := snapshot.Model + "\x00" + snapshot.Workspace
		protectsLatest := eligible && !latestSuccessful[scope]
		if eligible {
			latestSuccessful[scope] = true
		}
		if pinned > 0 || position < opts.Keep || protectsLatest {
			continue
		}
		if !opts.OlderThan.IsZero() && !snapshot.CreatedAt.Before(opts.OlderThan) {
			continue
		}
		victims = append(victims, snapshot.SnapshotID)
	}
	return victims, nil
}

// pruneImpact computes, without deleting anything, the records and stored paths
// a prune of these snapshots would orphan.
func (c *CatalogDB) pruneImpact(snapshotIDs []string) (int, []string, error) {
	if len(snapshotIDs) == 0 {
		return 0, nil, nil
	}
	doomed := map[string]bool{}
	for _, id := range snapshotIDs {
		doomed[id] = true
	}

	// A record survives if any snapshot outside the doomed set still cites it.
	var paths []string
	seen := map[string]bool{}
	for _, snapshotID := range snapshotIDs {
		entries, err := c.SnapshotRecordEntries(snapshotID)
		if err != nil {
			return 0, nil, err
		}
		for _, entry := range entries {
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			survives, err := c.itemCitedOutside(entry.ID, doomed)
			if err != nil {
				return 0, nil, err
			}
			if !survives {
				paths = append(paths, entry.StoredPath)
			}
		}
	}
	return len(paths), paths, nil
}

// itemCitedOutside reports whether any collection other than the doomed set
// still holds this item.
func (c *CatalogDB) itemCitedOutside(itemID string, doomed map[string]bool) (bool, error) {
	rows, err := c.db.Query(`SELECT collection_id FROM collection_memberships WHERE item_id = ?`, itemID)
	if err != nil {
		return false, fmt.Errorf("read memberships for %q: %w", itemID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var collectionID string
		if err := rows.Scan(&collectionID); err != nil {
			return false, fmt.Errorf("scan membership for %q: %w", itemID, err)
		}
		if !doomed[collectionID] {
			return true, nil
		}
	}
	return false, rows.Err()
}

// deleteSnapshotIn removes one snapshot and returns the stored paths of the
// records it orphaned.
func deleteSnapshotIn(q dbtx, snapshotID string) ([]string, error) {
	// Preserve just enough history to distinguish a pruned earlier observation
	// from a model that never had a baseline.
	if _, err := q.Exec(`INSERT OR IGNORE INTO observe_snapshot_tombstones(snapshot_id,model,workspace,created_at,run_id,source,snapshot_order) SELECT snapshot_id,model,workspace,created_at,run_id,source,rowid FROM observe_snapshots WHERE snapshot_id=?`, snapshotID); err != nil {
		return nil, err
	}
	// Read the members before the memberships go, and decide each one's fate by
	// what still cites it afterwards.
	itemIDs, err := snapshotMemberIDs(q, snapshotID)
	if err != nil {
		return nil, err
	}

	if _, err := q.Exec(`DELETE FROM collection_memberships WHERE collection_id = ?`, snapshotID); err != nil {
		return nil, fmt.Errorf("delete snapshot %q memberships: %w", snapshotID, err)
	}

	var orphanPaths []string
	for _, itemID := range itemIDs {
		var remaining int
		if err := q.QueryRow(`SELECT COUNT(*) FROM collection_memberships WHERE item_id = ?`, itemID).Scan(&remaining); err != nil {
			return nil, fmt.Errorf("count memberships for %q: %w", itemID, err)
		}
		if remaining > 0 {
			continue
		}
		var storedPath string
		err := q.QueryRow(
			`SELECT stored_path FROM catalog_entries WHERE id = ? AND entry_type = 'observe' AND collection_type = 'item'`,
			itemID).Scan(&storedPath)
		if err != nil {
			// Not an observe item (or already gone): leave it alone. Prune owns
			// observations, not every entry that happens to sit in a collection.
			continue
		}
		if _, err := q.Exec(`DELETE FROM catalog_entries WHERE id = ?`, itemID); err != nil {
			return nil, fmt.Errorf("delete record %q: %w", itemID, err)
		}
		orphanPaths = append(orphanPaths, storedPath)
	}

	if _, err := q.Exec(`DELETE FROM catalog_entries WHERE id = ?`, snapshotID); err != nil {
		return nil, fmt.Errorf("delete snapshot entry %q: %w", snapshotID, err)
	}
	if _, err := q.Exec(`DELETE FROM snapshot_pins WHERE snapshot_id=?`, snapshotID); err != nil {
		return nil, err
	}
	if _, err := q.Exec(`DELETE FROM observe_snapshots WHERE snapshot_id = ?`, snapshotID); err != nil {
		return nil, fmt.Errorf("delete snapshot %q: %w", snapshotID, err)
	}
	return orphanPaths, nil
}

// prunableRawFile reports whether a stored path may be unlinked: it must lie
// under the data dir's raw/ tree. Pruning that reclaims no disk is most of the
// point missed, but a path-handling bug must not be able to unlink anything
// outside pudl's own staging area.
func prunableRawFile(storedPath, dataDir string) bool {
	if storedPath == "" || dataDir == "" {
		return false
	}
	rawRoot, err := filepath.Abs(filepath.Join(dataDir, "raw"))
	if err != nil {
		return false
	}
	absolute, err := filepath.Abs(storedPath)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absolute, rawRoot+string(filepath.Separator))
}

// snapshotMemberIDs lists the item IDs a snapshot holds. The rows are closed
// before returning, so the caller can modify memberships on the same q.
func snapshotMemberIDs(q dbtx, snapshotID string) ([]string, error) {
	rows, err := q.Query(`SELECT item_id FROM collection_memberships WHERE collection_id = ?`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("read snapshot %q members: %w", snapshotID, err)
	}
	defer rows.Close()
	var itemIDs []string
	for rows.Next() {
		var itemID string
		if err := rows.Scan(&itemID); err != nil {
			return nil, fmt.Errorf("scan snapshot %q member: %w", snapshotID, err)
		}
		itemIDs = append(itemIDs, itemID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read snapshot %q members: %w", snapshotID, err)
	}
	return itemIDs, nil
}

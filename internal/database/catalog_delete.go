package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeletePlan describes durable deletion. Files may be reclaimed only after the
// transaction commits and while holding the workspace artifact lock.
type DeletePlan struct {
	Entry        CatalogEntry
	DeletedItems []CatalogEntry
}

// DeleteEntriesAtomic removes memberships and entries together. Shared items
// survive a cascading collection deletion.
func (c *CatalogDB) DeleteEntriesAtomic(id string, cascade bool) (DeletePlan, error) {
	return c.DeleteEntriesAtomicContext(context.Background(), id, cascade)
}

func (c *CatalogDB) DeleteEntriesAtomicContext(ctx context.Context, id string, cascade bool) (DeletePlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var plan DeletePlan
	err := c.WithCatalogTxContext(ctx, func(tx *CatalogTx) error {
		entry, err := tx.GetEntry(id)
		if err != nil {
			return err
		}
		if err := refuseProtectedEvidenceIn(tx.q, id); err != nil {
			return err
		}
		plan.Entry = *entry
		var itemIDs []string
		if entry.CollectionType != nil && *entry.CollectionType == "collection" {
			if cascade {
				itemIDs, err = snapshotMemberIDs(tx.q, id)
				if err != nil {
					return err
				}
			}
			if _, err := tx.q.Exec(`DELETE FROM collection_memberships WHERE collection_id = ?`, id); err != nil {
				return err
			}
		} else {
			if _, err := tx.q.Exec(`DELETE FROM collection_memberships WHERE item_id = ?`, id); err != nil {
				return err
			}
		}
		for _, itemID := range itemIDs {
			var remaining int
			if err := tx.q.QueryRow(`SELECT COUNT(*) FROM collection_memberships WHERE item_id = ?`, itemID).Scan(&remaining); err != nil {
				return err
			}
			if remaining != 0 {
				continue
			}
			item, err := tx.GetEntry(itemID)
			if err != nil {
				return err
			}
			if _, err := tx.q.Exec(`DELETE FROM catalog_entries WHERE id = ?`, itemID); err != nil {
				return err
			}
			plan.DeletedItems = append(plan.DeletedItems, *item)
		}
		if _, err := tx.q.Exec(`DELETE FROM catalog_entries WHERE id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.q.Exec(`INSERT OR IGNORE INTO observe_snapshot_tombstones(snapshot_id,model,workspace,created_at,run_id,source,snapshot_order) SELECT snapshot_id,model,workspace,created_at,run_id,source,rowid FROM observe_snapshots WHERE snapshot_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.q.Exec(`DELETE FROM snapshot_pins WHERE snapshot_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.q.Exec(`DELETE FROM observe_snapshots WHERE snapshot_id = ?`, id); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return DeletePlan{}, err
	}
	return plan, nil
}

// RemoveCommittedOrphan removes one unreferenced artifact in raw/ or metadata/.
// The caller must hold the artifact lock from before its deletion transaction
// through this call, so publishers cannot race the reference check and unlink.
func (c *CatalogDB) RemoveCommittedOrphan(path string) (bool, error) {
	return c.removeCommittedOrphanAt(path, filepath.Join(c.configDir, "data"))
}

func (c *CatalogDB) removeCommittedOrphanAt(path, dataDir string) (bool, error) {
	if path == "" {
		return false, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	allowed := false
	for _, name := range []string{"raw", "metadata"} {
		root, err := filepath.Abs(filepath.Join(dataDir, name))
		if err != nil {
			return false, err
		}
		if strings.HasPrefix(absolute, root+string(filepath.Separator)) {
			allowed = true
		}
	}
	if !allowed {
		return false, fmt.Errorf("refusing artifact cleanup outside workspace: %s", path)
	}
	var refs int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM catalog_entries WHERE stored_path = ? OR metadata_path = ?`, path, path).Scan(&refs); err != nil {
		return false, err
	}
	if refs != 0 {
		return false, nil
	}
	if err := os.Remove(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// Root is the workspace root owning this catalog and its artifact lock.
func (c *CatalogDB) Root() string { return c.configDir }

// refuseProtectedEvidenceIn prevents explicit entry deletion from bypassing
// retention: deleting an item would otherwise erase its membership in every
// snapshot, including immutable evidence used by another pending approval.
func refuseProtectedEvidenceIn(q dbtx, id string) error {
	var protected string
	err := q.QueryRow(`SELECT s.snapshot_id FROM observe_snapshots s
 WHERE (s.snapshot_id=? OR EXISTS(SELECT 1 FROM collection_memberships m WHERE m.collection_id=s.snapshot_id AND m.item_id=?))
 AND (
  EXISTS(SELECT 1 FROM snapshot_pins p WHERE p.snapshot_id=s.snapshot_id AND (p.expires_at IS NULL OR julianday(p.expires_at)>julianday('now')))
  OR (
   s.source IN ('mu-observe','ewe','ingest-observe')
 AND NOT EXISTS(SELECT 1 FROM snapshot_reuse_blocks b WHERE b.snapshot_id=s.snapshot_id)
   AND EXISTS(SELECT 1 FROM runs r WHERE r.run_id=s.run_id AND r.model=s.model AND r.completion_status='succeeded')
   AND NOT EXISTS(SELECT 1 FROM observe_snapshots newer JOIN runs r ON r.run_id=newer.run_id AND r.model=newer.model
    WHERE newer.model=s.model AND newer.workspace=s.workspace AND r.completion_status='succeeded'
    AND newer.source IN ('mu-observe','ewe','ingest-observe')
 AND NOT EXISTS(SELECT 1 FROM snapshot_reuse_blocks b WHERE b.snapshot_id=newer.snapshot_id)
    AND (newer.created_at>s.created_at OR (newer.created_at=s.created_at AND newer.rowid>s.rowid)))
  )
 ) LIMIT 1`, id, id).Scan(&protected)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("cannot delete protected snapshot evidence %q (snapshot %q): release its owners or record a newer successful observation first", id, protected)
}

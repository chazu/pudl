package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ScrubbedEntry is an entry after the sensitive scrub rewrote its files: the
// re-derived ID (equal to OldID when the ID is not content-derived) and the
// columns that describe the new files.
type ScrubbedEntry struct {
	OldID, NewID   string
	OldContentHash string // guards against a concurrent change since planning
	ContentHash    string
	StoredPath     string
	MetadataPath   string
	SizeBytes      int64
	ResourceID     *string // nil keeps the current resource_id
}

// RekeyScrubbedEntry moves an entry to its re-derived ID and rewritten files,
// and rewrites every column that stores the old ID. When NewID already names
// another entry (same redacted content), the old row merges into it: its
// references move to the survivor and the row is deleted. It reports whether
// it merged. Projection state is the caller's: it re-projects (see
// projection.Correct).
func (t *CatalogTx) RekeyScrubbedEntry(s ScrubbedEntry) (bool, error) {
	now := formatCatalogTime(time.Now())
	if s.NewID == s.OldID {
		res, err := t.q.Exec(`UPDATE catalog_entries SET content_hash = ?, stored_path = ?, metadata_path = ?,
			size_bytes = ?, resource_id = COALESCE(?, resource_id), updated_at = ? WHERE id = ? AND content_hash IS ?`,
			s.ContentHash, s.StoredPath, s.MetadataPath, s.SizeBytes, s.ResourceID, now, s.OldID, s.OldContentHash)
		return false, expectOneRow(res, err, s.OldID)
	}
	var snapshot bool
	if err := t.q.QueryRow(`SELECT EXISTS(SELECT 1 FROM observe_snapshots WHERE snapshot_id = ?)`, s.OldID).Scan(&snapshot); err != nil {
		return false, err
	}
	if snapshot {
		return false, fmt.Errorf("re-key %s: snapshot IDs are pre-allocated and never re-derived", s.OldID)
	}
	var survivor string
	err := t.q.QueryRow(`SELECT id FROM catalog_entries WHERE id = ?`, s.NewID).Scan(&survivor)
	merged := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if merged {
		var current sql.NullString
		if err := t.q.QueryRow(`SELECT content_hash FROM catalog_entries WHERE id = ?`, s.OldID).Scan(&current); err != nil {
			return false, fmt.Errorf("merge %s: %w", s.OldID, err)
		}
		if current.String != s.OldContentHash {
			return false, fmt.Errorf("entry %s changed since the scrub was planned; rerun it", s.OldID)
		}
	} else {
		res, err := t.q.Exec(`UPDATE catalog_entries SET id = ?, item_id = CASE WHEN item_id = ? THEN ? ELSE item_id END,
			content_hash = ?, stored_path = ?, metadata_path = ?, size_bytes = ?,
			resource_id = COALESCE(?, resource_id), updated_at = ? WHERE id = ? AND content_hash IS ?`,
			s.NewID, s.OldID, s.NewID, s.ContentHash, s.StoredPath, s.MetadataPath, s.SizeBytes,
			s.ResourceID, now, s.OldID, s.OldContentHash)
		if err := expectOneRow(res, err, s.OldID); err != nil {
			return false, err
		}
	}
	if err := moveEntryReferences(t.q, s.OldID, s.NewID); err != nil {
		return false, err
	}
	if merged {
		if _, err := t.q.Exec(`DELETE FROM catalog_entries WHERE id = ?`, s.OldID); err != nil {
			return false, err
		}
	}
	return merged, nil
}

// moveEntryReferences rewrites the relationship columns that store an entry
// ID. A row the target already has wins over the moved duplicate.
func moveEntryReferences(q dbtx, oldID, newID string) error {
	for _, ref := range []struct{ table, column string }{
		{"collection_memberships", "item_id"},
		{"collection_memberships", "collection_id"},
		{"item_schemas", "item_id"},
		// Policy results carry no record value; only the entry they describe
		// moves. Their facts' provenance keeps the old ID, which redirects.
		{"policy_state", "entry_id"},
	} {
		if _, err := q.Exec(fmt.Sprintf(`UPDATE OR IGNORE %s SET %s = ? WHERE %s = ?`, ref.table, ref.column, ref.column), newID, oldID); err != nil {
			return fmt.Errorf("re-key %s.%s: %w", ref.table, ref.column, err)
		}
		if _, err := q.Exec(fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, ref.table, ref.column), oldID); err != nil {
			return fmt.Errorf("re-key %s.%s: %w", ref.table, ref.column, err)
		}
	}
	// Import items are named <collection id>_item_<n>.
	prefix := oldID + "_item_"
	_, err := q.Exec(`UPDATE catalog_entries SET origin = ? || substr(origin, ?) WHERE substr(origin, 1, ?) = ?`,
		newID+"_item_", len(prefix)+1, len(prefix), prefix)
	return err
}

func expectOneRow(res sql.Result, err error, id string) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("entry %s changed since the scrub was planned; rerun it", id)
	}
	return nil
}

// ReportBlob is one stored report or legacy approval request.
type ReportBlob struct {
	Kind string // run_report | run_set_report | run_approval | run_set_approval
	Key  string
	JSON []byte
}

var reportBlobTables = map[string]struct{ table, key, column string }{
	"run_report":       {"run_reports", "run_id", "report_json"},
	"run_set_report":   {"run_set_reports", "run_set_id", "report_json"},
	"run_approval":     {"run_approvals", "run_id", "request_json"},
	"run_set_approval": {"run_set_approvals", "run_set_id", "request_json"},
}

// ReportBlobKinds lists the report kinds in a stable order.
var ReportBlobKinds = []string{"run_report", "run_set_report", "run_approval", "run_set_approval"}

// EachReportBlob streams every stored report and approval request to fn.
// The read holds a connection, so fn should not query c.
func (c *CatalogDB) EachReportBlob(ctx context.Context, fn func(ReportBlob) error) error {
	for _, kind := range ReportBlobKinds {
		t := reportBlobTables[kind]
		if ok, err := c.hasTable(t.table); err != nil || !ok {
			if err != nil {
				return err
			}
			continue
		}
		if err := c.eachReportBlobIn(ctx, kind, fn); err != nil {
			return err
		}
	}
	return nil
}

func (c *CatalogDB) eachReportBlobIn(ctx context.Context, kind string, fn func(ReportBlob) error) error {
	t := reportBlobTables[kind]
	rows, err := c.db.QueryContext(ctx, fmt.Sprintf(`SELECT %s, %s FROM %s ORDER BY %s`, t.key, t.column, t.table, t.key))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		blob := ReportBlob{Kind: kind}
		if err := rows.Scan(&blob.Key, &blob.JSON); err != nil {
			return err
		}
		if err := fn(blob); err != nil {
			return err
		}
	}
	return rows.Err()
}

// RewriteReportBlob re-reads one report inside the transaction and stores
// rewrite's result when it changed. Pins and evidence references are left as
// they are: SaveRunReport would reset them.
func (t *CatalogTx) RewriteReportBlob(kind, key string, rewrite func([]byte) ([]byte, bool, error)) (bool, error) {
	tbl, ok := reportBlobTables[kind]
	if !ok {
		return false, fmt.Errorf("unknown report kind %q", kind)
	}
	var payload []byte
	if err := t.q.QueryRow(fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`, tbl.column, tbl.table, tbl.key), key).Scan(&payload); err != nil {
		return false, fmt.Errorf("read %s %s: %w", kind, key, err)
	}
	updated, changed, err := rewrite(payload)
	if err != nil || !changed {
		return false, err
	}
	_, err = t.q.Exec(fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ?`, tbl.table, tbl.column, tbl.key), updated, key)
	return err == nil, err
}

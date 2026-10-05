package database

import (
	"fmt"
	"time"
)

const (
	SnapshotPinManual              = "manual"
	SnapshotPinApproval            = "approval"
	SnapshotPinReport              = "retained-report"
	DefaultReportEvidenceRetention = 30 * 24 * time.Hour
)

// SnapshotPin identifies an independent reason to preserve a snapshot.
type SnapshotPin struct {
	SnapshotID string     `json:"snapshot_id"`
	OwnerKind  string     `json:"owner_kind"`
	OwnerID    string     `json:"owner_id"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

func (c *CatalogDB) ensureSnapshotPins() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS snapshot_pins (snapshot_id TEXT NOT NULL, owner_kind TEXT NOT NULL, owner_id TEXT NOT NULL, expires_at TIMESTAMP, PRIMARY KEY(snapshot_id,owner_kind,owner_id))`,
		`CREATE TABLE IF NOT EXISTS report_snapshot_refs (run_id TEXT NOT NULL, snapshot_id TEXT NOT NULL, PRIMARY KEY(run_id,snapshot_id))`,
		`CREATE TABLE IF NOT EXISTS observe_snapshot_tombstones (snapshot_id TEXT PRIMARY KEY,model TEXT NOT NULL,workspace TEXT NOT NULL,created_at TIMESTAMP NOT NULL,run_id TEXT NOT NULL,source TEXT NOT NULL,snapshot_order INTEGER NOT NULL)`,
		// Existing booleans do not record their owner. Preserve conservatively as
		// manual pins, so an approval's release cannot erase historical retention.
		`INSERT OR IGNORE INTO snapshot_pins (snapshot_id,owner_kind,owner_id) SELECT snapshot_id,'manual','manual' FROM observe_snapshots WHERE retained != 0`,
	}
	for _, stmt := range statements {
		if _, err := c.db.Exec(stmt); err != nil {
			return fmt.Errorf("snapshot pins migration: %w", err)
		}
	}
	return c.backfillReportAndApprovalPins()
}

// SetSnapshotPin changes only one owner's pin. It cannot release another
// approval's evidence or an operator's manual retention.
func (c *CatalogDB) SetSnapshotPin(pin SnapshotPin, retain bool) error {
	return c.WithCatalogTx(func(tx *CatalogTx) error { return setSnapshotPinIn(tx.q, pin, retain) })
}

func setSnapshotPinIn(q dbtx, pin SnapshotPin, retain bool) error {
	if pin.SnapshotID == "" || pin.OwnerKind == "" || pin.OwnerID == "" {
		return fmt.Errorf("snapshot pin requires snapshot and owner identity")
	}
	var count int
	if err := q.QueryRow(`SELECT COUNT(*) FROM observe_snapshots WHERE snapshot_id = ?`, pin.SnapshotID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("retain observe snapshot %q: no such snapshot", pin.SnapshotID)
	}
	var expires any
	if pin.ExpiresAt != nil {
		expires = formatCatalogTime(pin.ExpiresAt.UTC())
	}
	if retain {
		if _, err := q.Exec(`INSERT INTO snapshot_pins(snapshot_id,owner_kind,owner_id,expires_at) VALUES(?,?,?,?) ON CONFLICT(snapshot_id,owner_kind,owner_id) DO UPDATE SET expires_at=excluded.expires_at`, pin.SnapshotID, pin.OwnerKind, pin.OwnerID, expires); err != nil {
			return err
		}
	} else if _, err := q.Exec(`DELETE FROM snapshot_pins WHERE snapshot_id=? AND owner_kind=? AND owner_id=?`, pin.SnapshotID, pin.OwnerKind, pin.OwnerID); err != nil {
		return err
	}
	return refreshSnapshotRetainedIn(q, pin.SnapshotID)
}

func refreshSnapshotRetainedIn(q dbtx, id string) error {
	_, err := q.Exec(`UPDATE observe_snapshots SET retained=EXISTS(SELECT 1 FROM snapshot_pins p WHERE p.snapshot_id=observe_snapshots.snapshot_id AND (p.expires_at IS NULL OR julianday(p.expires_at) > julianday(?))) WHERE snapshot_id=?`, formatCatalogTime(time.Now().UTC()), id)
	return err
}

// SnapshotPins returns active owners, making automatic retention inspectable.
func (c *CatalogDB) SnapshotPins(id string) ([]SnapshotPin, error) {
	if !c.hasSnapshotPins() {
		snapshot, err := c.GetObserveSnapshot(id)
		if err != nil {
			return nil, err
		}
		if snapshot != nil && snapshot.Retained {
			return []SnapshotPin{{SnapshotID: id, OwnerKind: SnapshotPinManual, OwnerID: "legacy"}}, nil
		}
		return nil, nil
	}

	rows, err := c.db.Query(`SELECT snapshot_id,owner_kind,owner_id,expires_at FROM snapshot_pins WHERE snapshot_id=? AND (expires_at IS NULL OR julianday(expires_at)>julianday(?)) ORDER BY owner_kind,owner_id`, id, formatCatalogTime(time.Now().UTC()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pins []SnapshotPin
	for rows.Next() {
		var pin SnapshotPin
		if err := rows.Scan(&pin.SnapshotID, &pin.OwnerKind, &pin.OwnerID, &pin.ExpiresAt); err != nil {
			return nil, err
		}
		pins = append(pins, pin)
	}
	return pins, rows.Err()
}

// backfillReportAndApprovalPins preserves existing report evidence and restores
// named owners for pending exact approvals. Legacy manual pins stay intact.
func (c *CatalogDB) backfillReportAndApprovalPins() error {
	rows, err := c.db.Query(`SELECT run_id,report_json,created_at FROM run_reports`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type report struct {
		id        string
		payload   []byte
		createdAt time.Time
	}
	var reports []report
	for rows.Next() {
		var r report
		if err := rows.Scan(&r.id, &r.payload, &r.createdAt); err != nil {
			rows.Close()
			return err
		}
		reports = append(reports, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range reports {
		if err := recordReportEvidenceUntilIn(c.db, r.id, r.payload, r.createdAt.Add(DefaultReportEvidenceRetention)); err != nil {
			return err
		}
	}
	rows, err = c.db.Query(`SELECT a.run_set_id,r.report_json FROM run_set_approvals a JOIN run_set_reports r ON r.run_set_id=a.run_set_id WHERE a.status='pending'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var approvals []report
	for rows.Next() {
		var r report
		if err := rows.Scan(&r.id, &r.payload); err != nil {
			rows.Close()
			return err
		}
		approvals = append(approvals, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range approvals {
		// The run-set projection names member run IDs; member reports carry the
		// immutable populate/binding snapshots that exact approval depends on.
		for _, r := range reports {
			members := reportFieldIDs(a.payload, "run_id")
			matches := false
			for _, id := range members {
				if id == r.id {
					matches = true
				}
			}
			if !matches {
				continue
			}
			for _, id := range reportSnapshotIDs(r.payload) {
				var exists int
				if err := c.db.QueryRow(`SELECT COUNT(*) FROM observe_snapshots WHERE snapshot_id=?`, id).Scan(&exists); err != nil {
					return err
				}
				if exists > 0 {
					if err := setSnapshotPinIn(c.db, SnapshotPin{SnapshotID: id, OwnerKind: SnapshotPinApproval, OwnerID: a.id}, true); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (c *CatalogDB) hasSnapshotPins() bool {
	var count int
	return c.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='snapshot_pins'`).Scan(&count) == nil && count > 0
}

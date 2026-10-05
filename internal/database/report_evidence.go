package database

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"
)

// ReportEvidence reports whether persisted report references can still load
// their raw snapshot. Recorded findings remain in report_json after pruning.
type ReportEvidence struct {
	SnapshotID string `json:"snapshot_id"`
	Status     string `json:"status"`
}

func reportSnapshotIDs(payload []byte) []string { return reportFieldIDs(payload, "snapshot_id") }

func reportFieldIDs(payload []byte, key string) []string {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var value any
	if dec.Decode(&value) != nil {
		return nil
	}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				if k == key {
					if id, ok := v.(string); ok && id != "" {
						seen[id] = true
					}
				}
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(value)
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func recordReportEvidenceIn(q dbtx, runID string, payload []byte) error {
	return recordReportEvidenceUntilIn(q, runID, payload, time.Now().UTC().Add(DefaultReportEvidenceRetention))
}

func recordReportEvidenceUntilIn(q dbtx, runID string, payload []byte, expires time.Time) error {
	if _, err := q.Exec(`DELETE FROM snapshot_pins WHERE owner_kind=? AND owner_id=?`, SnapshotPinReport, runID); err != nil {
		return err
	}
	if _, err := q.Exec(`DELETE FROM report_snapshot_refs WHERE run_id=?`, runID); err != nil {
		return err
	}
	for _, id := range reportSnapshotIDs(payload) {
		if _, err := q.Exec(`INSERT INTO report_snapshot_refs(run_id,snapshot_id) VALUES(?,?)`, runID, id); err != nil {
			return err
		}
		// Old or legacy reports may cite an already-unavailable snapshot. Keep the
		// reference for honest inspection, and pin only what still exists.
		if _, err := q.Exec(`INSERT INTO snapshot_pins(snapshot_id,owner_kind,owner_id,expires_at) SELECT snapshot_id,?,?,? FROM observe_snapshots WHERE snapshot_id=?`, SnapshotPinReport, runID, formatCatalogTime(expires.UTC()), id); err != nil {
			return err
		}
	}
	_, err := q.Exec(`UPDATE observe_snapshots SET retained=EXISTS(SELECT 1 FROM snapshot_pins p WHERE p.snapshot_id=observe_snapshots.snapshot_id AND (p.expires_at IS NULL OR julianday(p.expires_at)>julianday(?)))`, formatCatalogTime(time.Now().UTC()))
	return err
}

func (c *CatalogDB) RunReportEvidence(runID string) ([]ReportEvidence, error) {
	var refTable int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='report_snapshot_refs'`).Scan(&refTable); err != nil {
		return nil, err
	}
	if refTable == 0 {
		report, err := c.GetRunReport(runID)
		if err != nil || report == nil {
			return nil, err
		}
		var evidence []ReportEvidence
		for _, id := range reportSnapshotIDs(report.Report) {
			var exists int
			if err := c.db.QueryRow(`SELECT COUNT(*) FROM observe_snapshots WHERE snapshot_id=?`, id).Scan(&exists); err != nil {
				return nil, err
			}
			if exists == 0 {
				entry, err := c.GetEntry(id)
				if err == nil && entry != nil {
					exists = 1
				}
			}
			status := "pruned"
			if exists > 0 {
				status = "available"
			}
			evidence = append(evidence, ReportEvidence{SnapshotID: id, Status: status})
		}
		return evidence, nil
	}

	rows, err := c.db.Query(`SELECT r.snapshot_id,CASE WHEN s.snapshot_id IS NULL AND e.id IS NULL THEN 'pruned' ELSE 'available' END FROM report_snapshot_refs r LEFT JOIN observe_snapshots s ON s.snapshot_id=r.snapshot_id LEFT JOIN catalog_entries e ON e.id=r.snapshot_id WHERE r.run_id=? ORDER BY r.snapshot_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ReportEvidence
	for rows.Next() {
		var item ReportEvidence
		if err := rows.Scan(&item.SnapshotID, &item.Status); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

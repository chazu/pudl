package database

import (
	"database/sql"
	"fmt"
	"time"
)

// RunReportRecord stores the structured report emitted by a completed or
// interrupted pudl run. The JSON payload is owned by cmd so the catalog does
// not duplicate the report's schema.
type RunReportRecord struct {
	RunID     string
	Model     string
	Report    []byte
	CreatedAt time.Time
}

func (c *CatalogDB) ensureRunReportsTable() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS run_reports (
		run_id TEXT PRIMARY KEY,
		model TEXT NOT NULL DEFAULT '',
		report_json BLOB NOT NULL,
		created_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("run reports table migration: %w", err)
	}
	_, err = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_run_reports_created ON run_reports(created_at)`)
	if err != nil {
		return fmt.Errorf("run reports index migration: %w", err)
	}
	return nil
}

// SaveRunReport replaces the report for a run. A report is written after the
// phase data is known and before the run's terminal status is finalized.
func (c *CatalogDB) SaveRunReport(runID, model string, report []byte) error {
	if runID == "" {
		return fmt.Errorf("save run report: empty run id")
	}
	if len(report) == 0 {
		return fmt.Errorf("save run report %q: empty report", runID)
	}
	_, err := c.db.Exec(`INSERT INTO run_reports (run_id, model, report_json, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET model = excluded.model, report_json = excluded.report_json, created_at = excluded.created_at`,
		runID, model, report, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save run report %q: %w", runID, err)
	}
	return nil
}

// GetRunReport returns one report, or nil when no report exists for runID.
func (c *CatalogDB) GetRunReport(runID string) (*RunReportRecord, error) {
	row := c.db.QueryRow(`SELECT run_id, model, report_json, created_at FROM run_reports WHERE run_id = ?`, runID)
	var out RunReportRecord
	if err := row.Scan(&out.RunID, &out.Model, &out.Report, &out.CreatedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get run report %q: %w", runID, err)
	}
	return &out, nil
}

// LatestRunReport returns the newest stored report, or nil when none exists.
func (c *CatalogDB) LatestRunReport() (*RunReportRecord, error) {
	row := c.db.QueryRow(`SELECT run_id, model, report_json, created_at FROM run_reports ORDER BY created_at DESC, run_id DESC LIMIT 1`)
	var out RunReportRecord
	if err := row.Scan(&out.RunID, &out.Model, &out.Report, &out.CreatedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get latest run report: %w", err)
	}
	return &out, nil
}

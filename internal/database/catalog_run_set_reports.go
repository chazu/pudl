package database

import (
	"database/sql"
	"fmt"
	"time"
)

type RunSetReportRecord struct {
	RunSetID  string
	Report    []byte
	CreatedAt time.Time
}

func (c *CatalogDB) ensureRunSetReportsTable() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS run_set_reports (
		run_set_id TEXT PRIMARY KEY,
		report_json BLOB NOT NULL,
		created_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("run-set reports table migration: %w", err)
	}
	_, err = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_run_set_reports_created ON run_set_reports(created_at)`)
	if err != nil {
		return fmt.Errorf("run-set reports index migration: %w", err)
	}
	return nil
}

func (c *CatalogDB) SaveRunSetReport(runSetID string, report []byte) error {
	return saveRunSetReportIn(c.db, runSetID, report)
}

func saveRunSetReportIn(q dbtx, runSetID string, report []byte) error {
	if runSetID == "" {
		return fmt.Errorf("save run-set report: empty run-set id")
	}
	if len(report) == 0 {
		return fmt.Errorf("save run-set report %q: empty report", runSetID)
	}
	_, err := q.Exec(`INSERT INTO run_set_reports (run_set_id, report_json, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(run_set_id) DO UPDATE SET report_json = excluded.report_json, created_at = excluded.created_at`,
		runSetID, report, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save run-set report %q: %w", runSetID, err)
	}
	return nil
}

func (c *CatalogDB) GetRunSetReport(runSetID string) (*RunSetReportRecord, error) {
	row := c.db.QueryRow(`SELECT run_set_id, report_json, created_at FROM run_set_reports WHERE run_set_id = ?`, runSetID)
	var record RunSetReportRecord
	if err := row.Scan(&record.RunSetID, &record.Report, &record.CreatedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get run-set report %q: %w", runSetID, err)
	}
	return &record, nil
}

func (c *CatalogDB) LatestRunSetReport() (*RunSetReportRecord, error) {
	row := c.db.QueryRow(`SELECT run_set_id, report_json, created_at FROM run_set_reports ORDER BY created_at DESC, run_set_id DESC LIMIT 1`)
	var record RunSetReportRecord
	if err := row.Scan(&record.RunSetID, &record.Report, &record.CreatedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get latest run-set report: %w", err)
	}
	return &record, nil
}

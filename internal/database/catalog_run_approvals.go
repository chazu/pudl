package database

import (
	"database/sql"
	"fmt"
	"time"
)

// RunApprovalRecord is the durable mutation gate for a converge run.
type RunApprovalRecord struct {
	RunID      string
	Model      string
	Request    []byte
	Status     string
	CreatedAt  time.Time
	ResolvedAt *time.Time
}

func (c *CatalogDB) ensureRunApprovalsTable() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS run_approvals (
		run_id TEXT PRIMARY KEY,
		model TEXT NOT NULL,
		request_json BLOB NOT NULL,
		status TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("run approvals table migration: %w", err)
	}
	_, err = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_run_approvals_status ON run_approvals(status, created_at)`)
	if err != nil {
		return fmt.Errorf("run approvals index migration: %w", err)
	}
	return nil
}

func (c *CatalogDB) SaveRunApproval(runID, model string, request []byte) error {
	if runID == "" || model == "" || len(request) == 0 {
		return fmt.Errorf("save run approval: run id, model, and request are required")
	}
	_, err := c.db.Exec(`INSERT INTO run_approvals (run_id, model, request_json, status, created_at)
		VALUES (?, ?, ?, 'pending', ?)
		ON CONFLICT(run_id) DO UPDATE SET model = excluded.model, request_json = excluded.request_json, status = 'pending', created_at = excluded.created_at, resolved_at = NULL`,
		runID, model, request, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save run approval %q: %w", runID, err)
	}
	return nil
}

func (c *CatalogDB) GetRunApproval(runID string) (*RunApprovalRecord, error) {
	row := c.db.QueryRow(`SELECT run_id, model, request_json, status, created_at, resolved_at FROM run_approvals WHERE run_id = ?`, runID)
	var out RunApprovalRecord
	var resolved sql.NullTime
	if err := row.Scan(&out.RunID, &out.Model, &out.Request, &out.Status, &out.CreatedAt, &resolved); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get run approval %q: %w", runID, err)
	}
	if resolved.Valid {
		value := resolved.Time
		out.ResolvedAt = &value
	}
	return &out, nil
}

func (c *CatalogDB) ResolveRunApproval(runID, status string) error {
	if status != "approved" && status != "rejected" {
		return fmt.Errorf("resolve run approval: invalid status %q", status)
	}
	result, err := c.db.Exec(`UPDATE run_approvals SET status = ?, resolved_at = ? WHERE run_id = ? AND status = 'pending'`, status, time.Now().UTC(), runID)
	if err != nil {
		return fmt.Errorf("resolve run approval %q: %w", runID, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("resolve run approval %q: %w", runID, err)
	}
	if n == 0 {
		return fmt.Errorf("resolve run approval %q: no pending approval", runID)
	}
	return nil
}

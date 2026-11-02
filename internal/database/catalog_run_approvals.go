package database

import "fmt"

// ensureRunApprovalsTable keeps the legacy converge approval table that the
// retired `--require-approval` workflow wrote. Nothing writes it any more; it
// stays so catalogs created before the removal open unchanged.
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

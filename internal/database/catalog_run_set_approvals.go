package database

import "fmt"

// ensureRunSetApprovalsTable keeps the legacy run-set approval table that the
// retired mutating run sets wrote. Nothing writes it any more; it stays so
// catalogs created before the removal open unchanged and keep their history.
func (c *CatalogDB) ensureRunSetApprovalsTable() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS run_set_approvals (
		run_set_id TEXT PRIMARY KEY,
		plan_digest TEXT NOT NULL,
		request_json BLOB NOT NULL,
		plan_json BLOB NOT NULL,
		status TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("run-set approvals table migration: %w", err)
	}
	_, err = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_run_set_approvals_status ON run_set_approvals(status, created_at)`)
	if err != nil {
		return fmt.Errorf("run-set approvals index migration: %w", err)
	}
	return nil
}

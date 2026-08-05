package database

import (
	"database/sql"
	"fmt"
	"time"
)

// RunSetApprovalRecord is the durable exact-plan mutation gate. Plan contains
// only the redacted canonical approval projection; operational provider paths
// must never be written here.
type RunSetApprovalRecord struct {
	RunSetID   string
	PlanDigest string
	Request    []byte
	Plan       []byte
	Status     string
	CreatedAt  time.Time
	ResolvedAt *time.Time
}

// PendingRunSetMember is one mutation-required member whose reopened run state
// and pending member report commit with the exact approval record.
type PendingRunSetMember struct {
	RunID  string
	Model  string
	Report []byte
}

// PendingRunSetApproval is the complete short catalog step that creates an
// exact mutation gate. Operational references are absent from Plan; snapshot
// IDs name evidence that must remain pinned until resolution.
type PendingRunSetApproval struct {
	RunSetID    string
	PlanDigest  string
	Request     []byte
	Plan        []byte
	Report      []byte
	SnapshotIDs []string
	Members     []PendingRunSetMember
}

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

// SaveRunSetApproval creates one immutable pending approval. A run-set identity
// cannot be silently recycled with a replacement plan; stale/rejected plans
// require a new run-set ID and a fresh operator decision.
func (c *CatalogDB) SaveRunSetApproval(runSetID, planDigest string, request, plan []byte) error {
	return saveRunSetApprovalIn(c.db, runSetID, planDigest, request, plan)
}

func saveRunSetApprovalIn(q dbtx, runSetID, planDigest string, request, plan []byte) error {
	if runSetID == "" || planDigest == "" || len(request) == 0 || len(plan) == 0 {
		return fmt.Errorf("save run-set approval: run-set id, plan digest, request, and plan are required")
	}
	_, err := q.Exec(`INSERT INTO run_set_approvals
		(run_set_id, plan_digest, request_json, plan_json, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)`,
		runSetID, planDigest, request, plan, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save run-set approval %q: %w", runSetID, err)
	}
	return nil
}

// SavePendingRunSetApproval atomically reopens mutation-required member runs,
// writes their pending reports, pins every immutable snapshot, and commits the
// canonical plan plus its pending run-set report. No external work occurs in
// this transaction.
func (c *CatalogDB) SavePendingRunSetApproval(pending PendingRunSetApproval) error {
	if pending.RunSetID == "" || pending.PlanDigest == "" || len(pending.Request) == 0 || len(pending.Plan) == 0 || len(pending.Report) == 0 {
		return fmt.Errorf("save pending run-set approval: run-set id, plan digest, request, plan, and report are required")
	}
	for _, member := range pending.Members {
		if member.RunID == "" || member.Model == "" || len(member.Report) == 0 {
			return fmt.Errorf("save pending run-set approval %q: member run id, model, and report are required", pending.RunSetID)
		}
	}
	return c.WithCatalogTx(func(tx *CatalogTx) error {
		for _, member := range pending.Members {
			if err := prepareRunMutationIn(tx.q, member.RunID); err != nil {
				return err
			}
			if err := saveRunReportIn(tx.q, member.RunID, member.Model, member.Report); err != nil {
				return err
			}
		}
		for _, snapshotID := range pending.SnapshotIDs {
			if err := retainObserveSnapshotIn(tx.q, snapshotID, true); err != nil {
				return fmt.Errorf("retain mutation-plan snapshot %q: %w", snapshotID, err)
			}
		}
		if err := saveRunSetApprovalIn(tx.q, pending.RunSetID, pending.PlanDigest, pending.Request, pending.Plan); err != nil {
			return err
		}
		return saveRunSetReportIn(tx.q, pending.RunSetID, pending.Report)
	})
}

func (c *CatalogDB) GetRunSetApproval(runSetID string) (*RunSetApprovalRecord, error) {
	row := c.db.QueryRow(`SELECT run_set_id, plan_digest, request_json, plan_json,
		status, created_at, resolved_at FROM run_set_approvals WHERE run_set_id = ?`, runSetID)
	var record RunSetApprovalRecord
	var resolved sql.NullTime
	if err := row.Scan(&record.RunSetID, &record.PlanDigest, &record.Request, &record.Plan,
		&record.Status, &record.CreatedAt, &resolved); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get run-set approval %q: %w", runSetID, err)
	}
	if resolved.Valid {
		value := resolved.Time
		record.ResolvedAt = &value
	}
	return &record, nil
}

// ResolveRunSetApproval is a compare-and-swap over both status and plan
// identity. A caller that rebuilt a different plan cannot approve the older
// pending record, even if another process races the same approval.
func (c *CatalogDB) ResolveRunSetApproval(runSetID, expectedDigest, status string) error {
	return resolveRunSetApprovalIn(c.db, runSetID, expectedDigest, status)
}

func resolveRunSetApprovalIn(q dbtx, runSetID, expectedDigest, status string) error {
	if status != "approved" && status != "rejected" && status != "stale" {
		return fmt.Errorf("resolve run-set approval: invalid status %q", status)
	}
	result, err := q.Exec(`UPDATE run_set_approvals SET status = ?, resolved_at = ?
		WHERE run_set_id = ? AND plan_digest = ? AND status = 'pending'`,
		status, time.Now().UTC(), runSetID, expectedDigest)
	if err != nil {
		return fmt.Errorf("resolve run-set approval %q: %w", runSetID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("resolve run-set approval %q: %w", runSetID, err)
	}
	if affected == 0 {
		return fmt.Errorf("resolve run-set approval %q: no matching pending exact plan", runSetID)
	}
	return nil
}

// ApproveRunSetPlan atomically consumes the pending exact-plan gate and makes
// the approved/running run-set report visible. A racing or stale digest changes
// neither surface.
func (c *CatalogDB) ApproveRunSetPlan(runSetID, expectedDigest string, report []byte) error {
	if len(report) == 0 {
		return fmt.Errorf("approve run-set plan %q: empty report", runSetID)
	}
	return c.WithCatalogTx(func(tx *CatalogTx) error {
		if err := resolveRunSetApprovalIn(tx.q, runSetID, expectedDigest, "approved"); err != nil {
			return err
		}
		return saveRunSetReportIn(tx.q, runSetID, report)
	})
}

package database

import (
	"database/sql"
	"fmt"
	"time"
)

// RunRecord is the durable audit row for one `pudl run` invocation.
//
// Its purpose is to make an incomplete run *visible*: a run that dies before
// writing its verdict leaves an unfinished row, so the model's recorded status
// is known to predate it.
//
// Outcome, NeedsVerification, Applies and Scoped are legacy columns written
// only by the retired convergence workflow; they are read so old rows stay
// inspectable, and new runs leave them at their defaults.
type RunRecord struct {
	RunID      string
	Model      string
	Mode       string
	StartedAt  time.Time
	FinishedAt *time.Time
	// Verdict is the status the run persisted, or "" when it wrote none.
	Verdict string
	// Outcome is the convergence outcome string, for runs that converged.
	Outcome string
	// NeedsVerification marks a run that mutated the live system but could not
	// prove the result. It is what lets `unknown` be told apart from the
	// `unknown` of a resource nobody has ever observed.
	NeedsVerification bool
	// Note carries the reason behind a non-obvious verdict.
	Note string
	// Applies counts the successful mu applies this run performed. It is
	// incremented as each apply succeeds rather than written at the end, so a run
	// killed mid-converge still reports what it changed — which is the whole
	// point, since a crash-loop supervisor is what the durable apply budget
	// exists to bound.
	Applies int
	// Scoped marks a run restricted by `--only`. A scoped `clean` does not
	// generalize onto the model (see modelRowVerdict), so it must not reset the
	// apply budget either: alternating a converging scope with an oscillating one
	// would otherwise sustain exactly the unbounded apply rate the budget bounds.
	Scoped bool
	// CompletionStatus is the orchestration result, separate from the resource
	// verdict. Pure populate runs have no drift verdict but can still succeed and
	// own an eligible observation snapshot.
	CompletionStatus string
}

const (
	RunStatusRunning   = "running"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"
	RunStatusBlocked   = "blocked"
	RunStatusCancelled = "cancelled"
)

// Finished reports whether the run reached a terminal state. An unfinished row
// belonging to no live process is a run that died without recording a verdict.
func (r RunRecord) Finished() bool { return r.FinishedAt != nil }

// ensureRunsTable creates the run audit table. Idempotent, like every migration.
func (c *CatalogDB) ensureRunsTable() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMP NOT NULL,
			finished_at TIMESTAMP,
			verdict TEXT NOT NULL DEFAULT '',
			outcome TEXT NOT NULL DEFAULT '',
			needs_verification INTEGER NOT NULL DEFAULT 0,
			note TEXT NOT NULL DEFAULT '',
			applies INTEGER NOT NULL DEFAULT 0,
			scoped INTEGER NOT NULL DEFAULT 0,
			completion_status TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_model ON runs(model, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_unfinished ON runs(finished_at)`,
	}
	for _, statement := range statements {
		if _, err := c.db.Exec(statement); err != nil {
			return fmt.Errorf("runs table migration: %w", err)
		}
	}

	// A database created before the apply budget existed has the table but not
	// these columns. Added separately and idempotently, in the established style.
	for _, column := range []struct{ name, ddl string }{
		{"applies", "ALTER TABLE runs ADD COLUMN applies INTEGER NOT NULL DEFAULT 0"},
		{"scoped", "ALTER TABLE runs ADD COLUMN scoped INTEGER NOT NULL DEFAULT 0"},
		// Existing rows intentionally remain blank: without a status written by
		// the owning run they cannot prove successful completion and are therefore
		// ineligible for cross-resource observation reuse.
		{"completion_status", "ALTER TABLE runs ADD COLUMN completion_status TEXT NOT NULL DEFAULT ''"},
	} {
		exists, err := c.columnExists("runs", column.name)
		if err != nil {
			return fmt.Errorf("runs table migration: check column %s: %w", column.name, err)
		}
		if exists {
			continue
		}
		if _, err := c.db.Exec(column.ddl); err != nil {
			return fmt.Errorf("runs table migration: add column %s: %w", column.name, err)
		}
	}
	return nil
}

// StartRun records that a run began. Called before any phase, so that a run
// which never finishes is still discoverable.
func (c *CatalogDB) StartRun(runID, model, mode string) error {
	if runID == "" {
		return fmt.Errorf("start run: empty run id")
	}
	_, err := c.db.Exec(
		`INSERT INTO runs (run_id, model, mode, started_at, completion_status) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET model = excluded.model, mode = excluded.mode,
		 completion_status = excluded.completion_status`,
		runID, model, mode, time.Now().UTC(), RunStatusRunning,
	)
	if err != nil {
		return fmt.Errorf("start run %q: %w", runID, err)
	}
	return nil
}

// RunConclusion is what a terminated run concluded, recorded on its audit row.
type RunConclusion struct {
	CompletionStatus string
	Verdict          string
	Note             string
}

// FinishRun marks a run terminal and records what it concluded. A run that
// wrote no status passes an empty verdict — that is still a terminal run, and is
// distinguishable from one that never finished at all.
func (c *CatalogDB) FinishRun(runID string, conclusion RunConclusion) error {
	if runID == "" {
		return fmt.Errorf("finish run: empty run id")
	}
	if !terminalRunStatus(conclusion.CompletionStatus) {
		return fmt.Errorf("finish run %q: invalid terminal completion status %q", runID, conclusion.CompletionStatus)
	}
	result, err := c.db.Exec(
		`UPDATE runs SET finished_at = ?, completion_status = ?, verdict = ?, note = ?
		 WHERE run_id = ?`,
		time.Now().UTC(), conclusion.CompletionStatus, conclusion.Verdict, conclusion.Note, runID,
	)
	if err != nil {
		return fmt.Errorf("finish run %q: %w", runID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish run %q: %w", runID, err)
	}
	if affected == 0 {
		return fmt.Errorf("finish run %q: no such run", runID)
	}
	return nil
}

func terminalRunStatus(status string) bool {
	switch status {
	case RunStatusSucceeded, RunStatusFailed, RunStatusBlocked, RunStatusCancelled:
		return true
	default:
		return false
	}
}

// GetRun reads one run record, or nil when it does not exist.
func (c *CatalogDB) GetRun(runID string) (*RunRecord, error) {
	row := c.db.QueryRow(
		`SELECT run_id, model, mode, started_at, finished_at, verdict, outcome, needs_verification, note, applies, scoped, completion_status
		 FROM runs WHERE run_id = ?`, runID)
	record, err := scanRunRecord(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get run %q: %w", runID, err)
	}
	return &record, nil
}

// UnfinishedRuns returns runs that never reached a terminal state, newest first.
// For a given model when model is non-empty, otherwise across all models.
//
// A row here means a previous invocation died without recording a verdict, so
// whatever status its model currently carries predates that run and cannot be
// trusted.
func (c *CatalogDB) UnfinishedRuns(model string) ([]RunRecord, error) {
	query := `SELECT run_id, model, mode, started_at, finished_at, verdict, outcome, needs_verification, note, applies, scoped, completion_status
	          FROM runs WHERE finished_at IS NULL`
	args := []any{}
	if model != "" {
		query += ` AND model = ?`
		args = append(args, model)
	}
	query += ` ORDER BY started_at DESC`

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query unfinished runs: %w", err)
	}
	defer rows.Close()

	var records []RunRecord
	for rows.Next() {
		record, err := scanRunRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unfinished run: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// scanRunRecord maps one runs row. rowScanner lives in catalog_rows.go.
func scanRunRecord(row rowScanner) (RunRecord, error) {
	var (
		record     RunRecord
		finishedAt sql.NullTime
		needsVer   int
		scoped     int
	)
	err := row.Scan(
		&record.RunID, &record.Model, &record.Mode, &record.StartedAt, &finishedAt,
		&record.Verdict, &record.Outcome, &needsVer, &record.Note, &record.Applies, &scoped,
		&record.CompletionStatus,
	)
	if err != nil {
		return RunRecord{}, err
	}
	if finishedAt.Valid {
		finished := finishedAt.Time
		record.FinishedAt = &finished
	}
	record.NeedsVerification = needsVer != 0
	record.Scoped = scoped != 0
	return record, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

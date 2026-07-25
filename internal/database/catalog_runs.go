package database

import (
	"database/sql"
	"fmt"
	"time"
)

// RunRecord is the durable audit row for one `pudl run` invocation.
//
// Its purpose is to make an incomplete run *visible*. Before it existed, a run
// only ever wrote its verdict at the end, so a process that died mid-converge —
// or one whose re-observation failed after a successful apply — left the model
// instance row holding the previous run's status. A stale `clean` could survive
// a converge that had already mutated infrastructure.
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
}

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
			note TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_model ON runs(model, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_unfinished ON runs(finished_at)`,
	}
	for _, statement := range statements {
		if _, err := c.db.Exec(statement); err != nil {
			return fmt.Errorf("runs table migration: %w", err)
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
		`INSERT INTO runs (run_id, model, mode, started_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET model = excluded.model, mode = excluded.mode`,
		runID, model, mode, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("start run %q: %w", runID, err)
	}
	return nil
}

// FinishRun marks a run terminal and records what it concluded. A run that
// wrote no status passes an empty verdict — that is still a terminal run, and is
// distinguishable from one that never finished at all.
func (c *CatalogDB) FinishRun(runID, verdict, outcome string, needsVerification bool, note string) error {
	if runID == "" {
		return fmt.Errorf("finish run: empty run id")
	}
	result, err := c.db.Exec(
		`UPDATE runs SET finished_at = ?, verdict = ?, outcome = ?, needs_verification = ?, note = ?
		 WHERE run_id = ?`,
		time.Now().UTC(), verdict, outcome, boolToInt(needsVerification), note, runID,
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

// GetRun reads one run record, or nil when it does not exist.
func (c *CatalogDB) GetRun(runID string) (*RunRecord, error) {
	row := c.db.QueryRow(
		`SELECT run_id, model, mode, started_at, finished_at, verdict, outcome, needs_verification, note
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
	query := `SELECT run_id, model, mode, started_at, finished_at, verdict, outcome, needs_verification, note
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
	)
	err := row.Scan(
		&record.RunID, &record.Model, &record.Mode, &record.StartedAt, &finishedAt,
		&record.Verdict, &record.Outcome, &needsVer, &record.Note,
	)
	if err != nil {
		return RunRecord{}, err
	}
	if finishedAt.Valid {
		finished := finishedAt.Time
		record.FinishedAt = &finished
	}
	record.NeedsVerification = needsVer != 0
	return record, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

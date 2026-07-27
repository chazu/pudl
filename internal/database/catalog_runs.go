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
			note TEXT NOT NULL DEFAULT '',
			applies INTEGER NOT NULL DEFAULT 0,
			scoped INTEGER NOT NULL DEFAULT 0
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
		`INSERT INTO runs (run_id, model, mode, started_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET model = excluded.model, mode = excluded.mode`,
		runID, model, mode, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("start run %q: %w", runID, err)
	}
	return nil
}

// RunConclusion is what a terminated run concluded, recorded on its audit row.
// The apply count is deliberately absent: it is written per apply by RecordApply
// so it survives a run that never reaches here.
type RunConclusion struct {
	Verdict           string
	Outcome           string
	NeedsVerification bool
	Note              string
	// Scoped records that `--only` restricted the run, so a `clean` verdict here
	// covers a subset and must not reset the model's apply budget.
	Scoped bool
}

// FinishRun marks a run terminal and records what it concluded. A run that
// wrote no status passes an empty verdict — that is still a terminal run, and is
// distinguishable from one that never finished at all.
func (c *CatalogDB) FinishRun(runID string, conclusion RunConclusion) error {
	if runID == "" {
		return fmt.Errorf("finish run: empty run id")
	}
	result, err := c.db.Exec(
		`UPDATE runs SET finished_at = ?, verdict = ?, outcome = ?, needs_verification = ?, note = ?, scoped = ?
		 WHERE run_id = ?`,
		time.Now().UTC(), conclusion.Verdict, conclusion.Outcome,
		boolToInt(conclusion.NeedsVerification), conclusion.Note, boolToInt(conclusion.Scoped), runID,
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
		`SELECT run_id, model, mode, started_at, finished_at, verdict, outcome, needs_verification, note, applies, scoped
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
	query := `SELECT run_id, model, mode, started_at, finished_at, verdict, outcome, needs_verification, note, applies, scoped
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

// RecordApply increments the run's durable apply count.
//
// Called the moment an apply succeeds, before its manifest is recorded and
// before anything else in the iteration can fail. Writing the count only at
// FinishRun would leave it at zero for a run killed mid-converge — precisely the
// case the apply budget exists to bound, since the supervisor's next restart
// would then be granted a full budget again.
func (c *CatalogDB) RecordApply(runID string) error {
	if runID == "" {
		return fmt.Errorf("record apply: empty run id")
	}
	result, err := c.db.Exec(`UPDATE runs SET applies = applies + 1 WHERE run_id = ?`, runID)
	if err != nil {
		return fmt.Errorf("record apply for run %q: %w", runID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record apply for run %q: %w", runID, err)
	}
	if affected == 0 {
		return fmt.Errorf("record apply for run %q: no such run", runID)
	}
	return nil
}

// AppliesSinceLastClean sums the applies a model has performed since it was last
// verified clean without a scope restriction.
//
// It walks the model's runs newest-first and stops at the first unscoped `clean`
// verdict. Unfinished rows are counted: a run that died after two applies
// contributes two, which is why RecordApply increments per apply rather than at
// the end.
//
// A *scoped* clean does not stop the walk. It does not generalize onto the model
// (modelRowVerdict degrades it to `unknown` for the same reason), and treating it
// as a reset would let a scheduler alternating a converging `--only` scope with
// an oscillating one refill the budget every other run.
func (c *CatalogDB) AppliesSinceLastClean(model string) (int, error) {
	rows, err := c.db.Query(
		`SELECT verdict, scoped, applies FROM runs WHERE model = ? ORDER BY started_at DESC, rowid DESC`,
		model)
	if err != nil {
		return 0, fmt.Errorf("query apply history for %q: %w", model, err)
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var (
			verdict string
			scoped  int
			applies int
		)
		if err := rows.Scan(&verdict, &scoped, &applies); err != nil {
			return 0, fmt.Errorf("scan apply history for %q: %w", model, err)
		}
		if verdict == "clean" && scoped == 0 {
			break
		}
		total += applies
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read apply history for %q: %w", model, err)
	}
	return total, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

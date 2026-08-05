package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ObserveSnapshot is the durable contract for one observation: what was
// observed, by which run, from where, and when.
//
// It used to be a convention rather than an object — a JSON blob on disk whose
// identifying fields no query could reach, keyed in the catalog by the hash of
// its own bytes. Nothing recorded which model an observation belonged to, so
// "the current snapshot for this model" was not a question the catalog could
// answer, and nothing could be retained or pruned by any policy.
type ObserveSnapshot struct {
	// SnapshotID is both this row's key and the id of the catalog collection
	// entry holding the observed records. One identifier, allocated by the run
	// before it observes, so a failed ingest can still be named.
	SnapshotID string
	RunID      string
	// Model is the #SystemModel this observation was taken for, empty for a
	// standalone `pudl mu ingest-observe`.
	Model string
	// Workspace is where the run stood: the repo workspace name, or "global".
	// Recorded rather than resolved on read — it is history, and reading it from
	// the current context would silently relabel snapshots when a repo moves.
	Workspace string
	// Origin is the catalog ingest origin the records were stored under.
	Origin string
	// Source is how the observation was produced: mu-observe, ewe, ingest-observe.
	Source      string
	Targets     []string
	RecordCount int
	CreatedAt   time.Time
	// Retained pins a snapshot against pruning.
	Retained bool
}

// Snapshot sources — how the records in a snapshot were produced.
const (
	SnapshotSourceMuObserve     = "mu-observe"
	SnapshotSourceEwe           = "ewe"
	SnapshotSourceIngestObserve = "ingest-observe"
	// SnapshotSourceModelInstance is the model's own registration row, which
	// reuses the observe ingester as an implementation convenience. It is not an
	// observation of the live system, so it must never answer "what does this
	// model currently look like".
	SnapshotSourceModelInstance = "model-instance"
)

// observationSources are the sources that represent an actual observation of the
// live system. Listed positively rather than excluding model-instance, so a
// source added later has to decide which it is instead of defaulting into
// currentness.
var observationSources = []string{
	SnapshotSourceMuObserve,
	SnapshotSourceEwe,
	SnapshotSourceIngestObserve,
}

// ensureObserveSnapshotsTable creates the snapshot contract table. Idempotent,
// like every migration.
func (c *CatalogDB) ensureObserveSnapshotsTable() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS observe_snapshots (
			snapshot_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			workspace TEXT NOT NULL DEFAULT '',
			origin TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			targets TEXT NOT NULL DEFAULT '[]',
			record_count INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			retained INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_observe_snapshots_model ON observe_snapshots(model, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_observe_snapshots_run ON observe_snapshots(run_id)`,
	}
	for _, statement := range statements {
		if _, err := c.db.Exec(statement); err != nil {
			return fmt.Errorf("observe snapshots migration: %w", err)
		}
	}
	return nil
}

const observeSnapshotColumns = `snapshot_id, run_id, model, workspace, origin, source,
	targets, record_count, created_at, retained`

// RecordObserveSnapshot writes the snapshot contract row.
func (c *CatalogDB) RecordObserveSnapshot(snapshot ObserveSnapshot) error {
	return recordObserveSnapshotIn(c.db, snapshot)
}

// recordObserveSnapshotIn is the executor-parameterized form, so the contract row
// commits inside the same transaction as the collection entry and every
// membership: a snapshot describing records that were never stored is the partial
// state a later run would read as an observation.
func recordObserveSnapshotIn(q dbtx, snapshot ObserveSnapshot) error {
	if snapshot.SnapshotID == "" {
		return fmt.Errorf("record observe snapshot: empty snapshot id")
	}
	targets, err := json.Marshal(snapshot.Targets)
	if err != nil {
		return fmt.Errorf("record observe snapshot %q: marshal targets: %w", snapshot.SnapshotID, err)
	}
	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_, err = q.Exec(
		`INSERT INTO observe_snapshots (`+observeSnapshotColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.SnapshotID, snapshot.RunID, snapshot.Model, snapshot.Workspace,
		snapshot.Origin, snapshot.Source, string(targets), snapshot.RecordCount,
		formatCatalogTime(createdAt), boolToInt(snapshot.Retained),
	)
	if err != nil {
		return fmt.Errorf("record observe snapshot %q: %w", snapshot.SnapshotID, err)
	}
	return nil
}

// GetObserveSnapshot reads one snapshot, or nil when it has no contract row.
//
// A nil result is not the same as "no such snapshot": snapshots created before
// this table existed are still valid scopes, they simply carry no metadata.
func (c *CatalogDB) GetObserveSnapshot(snapshotID string) (*ObserveSnapshot, error) {
	row := c.db.QueryRow(
		`SELECT `+observeSnapshotColumns+` FROM observe_snapshots WHERE snapshot_id = ?`, snapshotID)
	snapshot, err := scanObserveSnapshot(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get observe snapshot %q: %w", snapshotID, err)
	}
	return &snapshot, nil
}

// CurrentObserveSnapshot returns the newest snapshot for a model, or nil when it
// has none.
//
// Currentness is derived rather than stored. A `current` column would be a
// second source of truth that can disagree with created_at, which is the shape
// of bug that let one run's records be attributed to another.
func (c *CatalogDB) CurrentObserveSnapshot(model string) (*ObserveSnapshot, error) {
	args := []any{model}
	placeholders := make([]string, len(observationSources))
	for i, source := range observationSources {
		placeholders[i] = "?"
		args = append(args, source)
	}
	row := c.db.QueryRow(
		`SELECT `+observeSnapshotColumns+` FROM observe_snapshots
		 WHERE model = ? AND source IN (`+strings.Join(placeholders, ", ")+`)
		 ORDER BY created_at DESC, rowid DESC LIMIT 1`, args...)
	snapshot, err := scanObserveSnapshot(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("current observe snapshot for %q: %w", model, err)
	}
	return &snapshot, nil
}

// LatestSuccessfulObserveSnapshot returns the newest real observation for the
// exact producer model and workspace whose owning run reached structured
// successful completion. Legacy rows without a completion status, unfinished
// runs, failed runs, registrations, and observations from another workspace are
// deliberately ineligible.
func (c *CatalogDB) LatestSuccessfulObserveSnapshot(model, workspace string) (*ObserveSnapshot, error) {
	return c.successfulObserveSnapshot(model, workspace, "")
}

// SuccessfulObserveSnapshotForRun returns the newest eligible observation
// owned by one exact run-set member. It is the current-run counterpart to
// LatestSuccessfulObserveSnapshot and never falls back to historical data.
func (c *CatalogDB) SuccessfulObserveSnapshotForRun(model, workspace, runID string) (*ObserveSnapshot, error) {
	if runID == "" {
		return nil, fmt.Errorf("successful observe snapshot for run: empty run id")
	}
	return c.successfulObserveSnapshot(model, workspace, runID)
}

// ObserveSnapshotByIDForRun reloads one already-pinned observation during
// exact-plan approval revalidation. The owning run may be in the deliberate
// running/pending-approval state, so eligibility was established before pinning
// and is not re-derived from its current completion status here.
func (c *CatalogDB) ObserveSnapshotByIDForRun(snapshotID, model, workspace, runID string) (*ObserveSnapshot, error) {
	if snapshotID == "" || model == "" || workspace == "" || runID == "" {
		return nil, fmt.Errorf("pinned observe snapshot requires snapshot, model, workspace, and run ids")
	}
	args := []any{snapshotID, model, workspace, runID}
	placeholders := make([]string, len(observationSources))
	for i, source := range observationSources {
		placeholders[i] = "?"
		args = append(args, source)
	}
	query := `SELECT ` + observeSnapshotColumns + ` FROM observe_snapshots
		WHERE snapshot_id = ? AND model = ? AND workspace = ? AND run_id = ?
		AND source IN (` + strings.Join(placeholders, ", ") + `) LIMIT 1`
	snapshot, err := scanObserveSnapshot(c.db.QueryRow(query, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pinned observe snapshot %q for run %q: %w", snapshotID, runID, err)
	}
	return &snapshot, nil
}

func (c *CatalogDB) successfulObserveSnapshot(model, workspace, runID string) (*ObserveSnapshot, error) {
	args := []any{model, workspace, RunStatusSucceeded}
	placeholders := make([]string, len(observationSources))
	for i, source := range observationSources {
		placeholders[i] = "?"
		args = append(args, source)
	}
	query := `SELECT ` + prefixedObserveSnapshotColumns("s") + `
		FROM observe_snapshots s
		JOIN runs r ON r.run_id = s.run_id AND r.model = s.model
		WHERE s.model = ? AND s.workspace = ? AND r.completion_status = ?
		AND s.source IN (` + strings.Join(placeholders, ", ") + `)`
	if runID != "" {
		query += ` AND s.run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY s.created_at DESC, s.rowid DESC LIMIT 1`

	snapshot, err := scanObserveSnapshot(c.db.QueryRow(query, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("successful observe snapshot for model %q in workspace %q: %w", model, workspace, err)
	}
	return &snapshot, nil
}

func prefixedObserveSnapshotColumns(alias string) string {
	columns := strings.Split(strings.ReplaceAll(observeSnapshotColumns, "\n", ""), ",")
	for i := range columns {
		columns[i] = alias + "." + strings.TrimSpace(columns[i])
	}
	return strings.Join(columns, ", ")
}

// ListObserveSnapshots returns snapshots newest-first, for one model when model
// is non-empty, otherwise across all models. A limit of 0 means no limit.
func (c *CatalogDB) ListObserveSnapshots(model string, limit int) ([]ObserveSnapshot, error) {
	query := `SELECT ` + observeSnapshotColumns + ` FROM observe_snapshots`
	var args []any
	if model != "" {
		query += ` WHERE model = ?`
		args = append(args, model)
	}
	query += ` ORDER BY created_at DESC, rowid DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list observe snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []ObserveSnapshot
	for rows.Next() {
		snapshot, err := scanObserveSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan observe snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

// RetainObserveSnapshot pins a snapshot against pruning, or releases it.
func (c *CatalogDB) RetainObserveSnapshot(snapshotID string, retained bool) error {
	return retainObserveSnapshotIn(c.db, snapshotID, retained)
}

func retainObserveSnapshotIn(q dbtx, snapshotID string, retained bool) error {
	result, err := q.Exec(
		`UPDATE observe_snapshots SET retained = ? WHERE snapshot_id = ?`,
		boolToInt(retained), snapshotID)
	if err != nil {
		return fmt.Errorf("retain observe snapshot %q: %w", snapshotID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("retain observe snapshot %q: %w", snapshotID, err)
	}
	if affected == 0 {
		return fmt.Errorf("retain observe snapshot %q: no such snapshot (snapshots created before the snapshot contract existed cannot be retained)", snapshotID)
	}
	return nil
}

func scanObserveSnapshot(row rowScanner) (ObserveSnapshot, error) {
	var (
		snapshot ObserveSnapshot
		targets  string
		retained int
	)
	err := row.Scan(
		&snapshot.SnapshotID, &snapshot.RunID, &snapshot.Model, &snapshot.Workspace,
		&snapshot.Origin, &snapshot.Source, &targets, &snapshot.RecordCount,
		&snapshot.CreatedAt, &retained,
	)
	if err != nil {
		return ObserveSnapshot{}, err
	}
	if targets != "" {
		if err := json.Unmarshal([]byte(targets), &snapshot.Targets); err != nil {
			return ObserveSnapshot{}, fmt.Errorf("parse targets for snapshot %q: %w", snapshot.SnapshotID, err)
		}
	}
	snapshot.Retained = retained != 0
	return snapshot, nil
}

// SnapshotRecordEntries returns the catalog entries a snapshot contains, in
// membership order.
//
// It reads collection_memberships rather than the legacy collection_id column:
// membership is the normalized relationship, and this is the first consumer
// written against it alone.
func (c *CatalogDB) SnapshotRecordEntries(snapshotID string) ([]CatalogEntry, error) {
	query := `SELECT ` + entrySelectVia("e", "m") + `
		FROM collection_memberships m
		JOIN catalog_entries e ON e.id = m.item_id
		WHERE m.collection_id = ?
		ORDER BY m.item_index`

	rows, err := c.db.Query(query, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("query snapshot %q records: %w", snapshotID, err)
	}
	defer rows.Close()

	var entries []CatalogEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan snapshot %q record: %w", snapshotID, err)
		}
		entries = append(entries, *entry)
	}
	return entries, rows.Err()
}

package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/errors"
)

// Schema-declared fact projection (`_pudl.facts`) writes facts whose source is
// ProjectionSourcePrefix + resource_id. projection_state records, per
// resource, which catalog entry those facts currently describe: the most
// recently OBSERVED one, which is not the max version — observe entries are all
// version 1, and a resource that reverts deduplicates to an old entry.

// ProjectionSourcePrefix marks facts owned by schema projection. Other writers
// must not use it: the next reconcile would close their facts.
const ProjectionSourcePrefix = "projection:"

// Projection state statuses.
const (
	ProjectionProjected = "projected"
	ProjectionSkipped   = "skipped" // identity unresolved: no stable source to own facts
)

// ProjectionMode says why a reconcile closes facts.
type ProjectionMode int

const (
	// ProjectionObservation: the world changed. Closed facts are invalidated
	// (valid_end set), so valid-time history shows the change.
	ProjectionObservation ProjectionMode = iota
	// ProjectionCorrection: our belief was wrong (spec change, reassignment,
	// deletion). Closed facts are retracted (tx_end set).
	ProjectionCorrection
)

// ProjectionState is one projection_state row.
type ProjectionState struct {
	ResourceID  string
	EntryID     string
	Schema      string
	Fingerprint string
	Status      string
	ObservedAt  int64
}

func (c *CatalogDB) ensureProjectionState() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS projection_state (
			resource_id TEXT PRIMARY KEY,
			entry_id    TEXT NOT NULL,
			schema      TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			status      TEXT NOT NULL,
			observed_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_projection_state_schema ON projection_state(schema)`,
		`CREATE INDEX IF NOT EXISTS idx_current_facts_source ON current_facts(source)`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			return fmt.Errorf("create projection state: %w", err)
		}
	}
	return nil
}

// ProjectionSource is the fact source owning a resource's projected facts.
func ProjectionSource(resourceID string) string {
	return ProjectionSourcePrefix + resourceID
}

// IsProjectionSource reports whether source is reserved for projection.
func IsProjectionSource(source string) bool {
	return strings.HasPrefix(source, ProjectionSourcePrefix)
}

func scanProjectionState(scan func(...any) error) (ProjectionState, error) {
	var s ProjectionState
	err := scan(&s.ResourceID, &s.EntryID, &s.Schema, &s.Fingerprint, &s.Status, &s.ObservedAt)
	return s, err
}

const selectProjectionStateSQL = `SELECT resource_id, entry_id, schema, fingerprint, status, observed_at FROM projection_state`

func getProjectionStateIn(q dbtx, resourceID string) (*ProjectionState, error) {
	s, err := scanProjectionState(q.QueryRow(selectProjectionStateSQL+` WHERE resource_id = ?`, resourceID).Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "read projection state", err)
	}
	return &s, nil
}

func listProjectionStateIn(q dbtx) ([]ProjectionState, error) {
	rows, err := q.Query(selectProjectionStateSQL + ` ORDER BY resource_id`)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "list projection state", err)
	}
	defer rows.Close()
	var out []ProjectionState
	for rows.Next() {
		s, err := scanProjectionState(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func putProjectionStateIn(q dbtx, s ProjectionState) error {
	if s.ObservedAt == 0 {
		s.ObservedAt = time.Now().Unix()
	}
	_, err := q.Exec(`INSERT INTO projection_state (resource_id, entry_id, schema, fingerprint, status, observed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(resource_id) DO UPDATE SET entry_id = excluded.entry_id, schema = excluded.schema,
			fingerprint = excluded.fingerprint, status = excluded.status, observed_at = excluded.observed_at`,
		s.ResourceID, s.EntryID, s.Schema, s.Fingerprint, s.Status, s.ObservedAt)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "write projection state", err)
	}
	return nil
}

func deleteProjectionStateIn(q dbtx, resourceID string) error {
	_, err := q.Exec(`DELETE FROM projection_state WHERE resource_id = ?`, resourceID)
	return err
}

// GetProjectionState, ListProjectionState, PutProjectionState and
// DeleteProjectionState are the transactional accessors.
func (t *CatalogTx) GetProjectionState(resourceID string) (*ProjectionState, error) {
	return getProjectionStateIn(t.q, resourceID)
}
func (t *CatalogTx) ListProjectionState() ([]ProjectionState, error) {
	return listProjectionStateIn(t.q)
}
func (t *CatalogTx) PutProjectionState(s ProjectionState) error {
	return putProjectionStateIn(t.q, s)
}
func (t *CatalogTx) DeleteProjectionState(resourceID string) error {
	return deleteProjectionStateIn(t.q, resourceID)
}

// ListProjectionState reads every row outside a transaction (read-only checks).
func (c *CatalogDB) ListProjectionState() ([]ProjectionState, error) {
	return listProjectionStateIn(c.db)
}

// ReconcileProjection makes source's current facts equal want, comparing by
// relation and canonical args. Facts no longer wanted are closed according to
// mode; wanted facts not current are added with valid_start = now. A wanted
// fact whose content-addressed ID collides with an already-closed fact (it was
// current once, at the same second) is re-added one second later, so a
// resource that reverts gets its facts back instead of a silent no-op.
func (t *CatalogTx) ReconcileProjection(source string, want []Fact, mode ProjectionMode) (added, closed int, err error) {
	if !IsProjectionSource(source) {
		return 0, 0, fmt.Errorf("reconcile: %q is not a projection source", source)
	}
	rows, err := t.q.Query(`SELECT id, relation, args FROM current_facts WHERE source = ?`, source)
	if err != nil {
		return 0, 0, errors.WrapError(errors.ErrCodeDatabaseError, "read projected facts", err)
	}
	current := map[string]string{}
	for rows.Next() {
		var id, relation, args string
		if err := rows.Scan(&id, &relation, &args); err != nil {
			rows.Close()
			return 0, 0, err
		}
		current[relation+"\x00"+canonicalizeJSON(args)] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	wanted := map[string]Fact{}
	for _, f := range want {
		wanted[f.Relation+"\x00"+canonicalizeJSON(f.Args)] = f
	}
	for key, id := range current {
		if _, keep := wanted[key]; keep {
			continue
		}
		if mode == ProjectionObservation {
			err = invalidateFactIn(t.q, id)
		} else {
			err = retractFactIn(t.q, id)
		}
		if err != nil {
			return added, closed, err
		}
		closed++
	}
	now := time.Now().Unix()
	for key, f := range wanted {
		if _, have := current[key]; have {
			continue
		}
		f.Source = source
		f.ID = ""
		for attempt := int64(0); ; attempt++ {
			f.ValidStart = now + attempt
			stored, err := addFactIn(t.q, f)
			if err != nil {
				return added, closed, err
			}
			if stored.ValidEnd == nil && stored.TxEnd == nil {
				break
			}
			if attempt == 3 {
				return added, closed, fmt.Errorf("reconcile %s: fact %s stays closed after retries", source, stored.ID)
			}
			f.ID = ""
		}
		added++
	}
	return added, closed, nil
}

// projectionOrphansIn returns state rows whose entry is gone or no longer has
// the row's resource ID (deleted, pruned, re-identified or reassigned).
func projectionOrphansIn(q dbtx) ([]ProjectionState, error) {
	rows, err := q.Query(`SELECT s.resource_id, s.entry_id, s.schema, s.fingerprint, s.status, s.observed_at
		FROM projection_state s LEFT JOIN catalog_entries e ON e.id = s.entry_id
		WHERE e.id IS NULL OR e.resource_id IS NULL OR e.resource_id != s.resource_id`)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "find orphaned projections", err)
	}
	defer rows.Close()
	var out []ProjectionState
	for rows.Next() {
		s, err := scanProjectionState(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// entriesNeedingProjectionIn returns, for resources of the given schemas that
// have no projection state yet, each resource's most recently imported entry.
func entriesNeedingProjectionIn(q dbtx, schemas []string) ([]CatalogEntry, error) {
	if len(schemas) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(schemas)), ",")
	args := make([]any, len(schemas))
	for i, s := range schemas {
		args[i] = s
	}
	rows, err := q.Query(`SELECT `+entrySelect("catalog_entries")+` FROM catalog_entries
		WHERE schema IN (`+placeholders+`) AND resource_id IS NOT NULL
		  AND resource_id NOT IN (SELECT resource_id FROM projection_state)
		ORDER BY resource_id, import_timestamp DESC, rowid DESC`, args...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "find unprojected entries", err)
	}
	defer rows.Close()
	var out []CatalogEntry
	last := ""
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		if *entry.ResourceID == last {
			continue
		}
		last = *entry.ResourceID
		out = append(out, *entry)
	}
	return out, rows.Err()
}

func (t *CatalogTx) ProjectionOrphans() ([]ProjectionState, error) { return projectionOrphansIn(t.q) }
func (c *CatalogDB) ProjectionOrphans() ([]ProjectionState, error) { return projectionOrphansIn(c.db) }
func (t *CatalogTx) EntriesNeedingProjection(schemas []string) ([]CatalogEntry, error) {
	return entriesNeedingProjectionIn(t.q, schemas)
}
func (c *CatalogDB) EntriesNeedingProjection(schemas []string) ([]CatalogEntry, error) {
	return entriesNeedingProjectionIn(c.db, schemas)
}

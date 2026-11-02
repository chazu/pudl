package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/idgen"
)

// The sensitive scrub (`pudl doctor --fix --only sensitive-scrub`) records what
// it redacted and which entry IDs it re-derived. These tables hold subjects,
// field paths and file paths only, never values.

func (c *CatalogDB) ensureSensitiveScrubTables() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS sensitive_scrubs (
			scrub_id     TEXT PRIMARY KEY,
			scrubbed_at  TIMESTAMP NOT NULL,
			compacted_at TIMESTAMP,
			summary      TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sensitive_redactions (
			scrub_id     TEXT NOT NULL,
			subject_kind TEXT NOT NULL,
			subject_id   TEXT NOT NULL,
			new_id       TEXT,
			merged       INTEGER NOT NULL DEFAULT 0,
			paths        TEXT NOT NULL,
			old_files    TEXT,
			PRIMARY KEY (scrub_id, subject_kind, subject_id)
		)`,
		`CREATE TABLE IF NOT EXISTS sensitive_id_redirects (
			old_id   TEXT PRIMARY KEY,
			new_id   TEXT NOT NULL,
			scrub_id TEXT NOT NULL
		)`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			return fmt.Errorf("sensitive scrub tables: %w", err)
		}
	}
	return nil
}

// SensitiveRedaction is one sensitive_redactions row: a subject the scrub
// rewrote (by its ID before the scrub), the paths it redacted (or
// "value-match") and the files it superseded.
type SensitiveRedaction struct {
	SubjectKind string
	SubjectID   string
	NewID       string // the entry's re-derived ID, when it changed
	Merged      bool
	Paths       []string
	OldFiles    []string
}

// RecordSensitiveScrub writes the scrub's row and its redactions, and a
// redirect for each entry whose ID changed. It runs in the rewrite transaction.
func (t *CatalogTx) RecordSensitiveScrub(scrubID, summary string, redactions []SensitiveRedaction) error {
	if _, err := t.q.Exec(`INSERT INTO sensitive_scrubs (scrub_id, scrubbed_at, summary) VALUES (?, ?, ?)`,
		scrubID, formatCatalogTime(time.Now().UTC()), summary); err != nil {
		return fmt.Errorf("record scrub: %w", err)
	}
	for _, r := range redactions {
		paths, _ := json.Marshal(r.Paths)
		var oldFiles any
		if len(r.OldFiles) > 0 {
			encoded, _ := json.Marshal(r.OldFiles)
			oldFiles = string(encoded)
		}
		var newID any
		if r.NewID != "" && r.NewID != r.SubjectID {
			newID = r.NewID
		}
		if _, err := t.q.Exec(`INSERT INTO sensitive_redactions (scrub_id, subject_kind, subject_id, new_id, merged, paths, old_files)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			scrubID, r.SubjectKind, r.SubjectID, newID, r.Merged, string(paths), oldFiles); err != nil {
			return fmt.Errorf("record redaction of %s: %w", r.SubjectID, err)
		}
		if newID != nil {
			if _, err := t.q.Exec(`INSERT INTO sensitive_id_redirects (old_id, new_id, scrub_id) VALUES (?, ?, ?)
				ON CONFLICT(old_id) DO UPDATE SET new_id = excluded.new_id, scrub_id = excluded.scrub_id`,
				r.SubjectID, r.NewID, scrubID); err != nil {
				return fmt.Errorf("record redirect of %s: %w", r.SubjectID, err)
			}
		}
	}
	return nil
}

// PendingScrubs reports work an earlier scrub left: the files it superseded
// (callers check which still exist) and the scrubs whose compaction has not
// finished. A catalog that predates the audit tables has none.
func (c *CatalogDB) PendingScrubs() (oldFiles, uncompacted []string, err error) {
	if ok, err := c.hasTable("sensitive_scrubs"); err != nil || !ok {
		return nil, nil, err
	}
	encoded, err := c.queryStrings(`SELECT old_files FROM sensitive_redactions WHERE old_files IS NOT NULL`)
	if err != nil {
		return nil, nil, err
	}
	for _, list := range encoded {
		var files []string
		if err := json.Unmarshal([]byte(list), &files); err != nil {
			return nil, nil, fmt.Errorf("decode scrub old_files: %w", err)
		}
		oldFiles = append(oldFiles, files...)
	}
	uncompacted, err = c.queryStrings(`SELECT scrub_id FROM sensitive_scrubs WHERE compacted_at IS NULL ORDER BY scrubbed_at`)
	return oldFiles, uncompacted, err
}

// IsArtifactReferenced reports whether any entry names path as its stored or
// metadata file.
func (c *CatalogDB) IsArtifactReferenced(path string) (bool, error) {
	var refs int
	err := c.db.QueryRow(`SELECT COUNT(*) FROM catalog_entries WHERE stored_path = ? OR metadata_path = ?`, path, path).Scan(&refs)
	return refs > 0, err
}

// ResolveEntryRedirect follows the scrub's redirects from id, a full entry ID
// or a proquint, to the entry's current ID. ok is false when no redirect
// names id, or when a proquint matches more than one redirect.
func (c *CatalogDB) ResolveEntryRedirect(id string) (string, bool, error) {
	if exists, err := c.hasTable("sensitive_id_redirects"); err != nil || !exists {
		return "", false, err
	}
	query, arg := `SELECT new_id FROM sensitive_id_redirects WHERE old_id = ?`, id
	if num, err := idgen.ProquintToNumber(id); err == nil {
		query, arg = `SELECT new_id FROM sensitive_id_redirects WHERE old_id LIKE ? LIMIT 2`, idgen.Uint32ToHash(num)+"%"
	}
	targets, err := c.queryStrings(query, arg)
	if err != nil || len(targets) != 1 {
		return "", false, err
	}
	// Follow later scrubs of the same entry; a redirect chain is acyclic
	// because each new ID hashes newly redacted content, but bound it anyway.
	current := targets[0]
	for range 16 {
		var next string
		err := c.db.QueryRow(`SELECT new_id FROM sensitive_id_redirects WHERE old_id = ?`, current).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			return current, true, nil
		}
		if err != nil {
			return "", false, err
		}
		current = next
	}
	return current, true, nil
}

// WithSecureCatalogTxContext runs fn in one immediate transaction on a
// connection with secure_delete on, so the pages it frees are zeroed.
func (c *CatalogDB) WithSecureCatalogTxContext(ctx context.Context, fn func(*CatalogTx) error) error {
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA secure_delete=ON"); err != nil {
		return fmt.Errorf("enable secure_delete: %w", err)
	}
	defer conn.ExecContext(context.Background(), "PRAGMA secure_delete=OFF") //nolint:errcheck // best effort on a pooled connection
	return runCatalogTx(ctx, conn, fn)
}

// RebuildFactsFTS rebuilds the facts search index, dropping the tokens of
// deleted rows that FTS5 segments otherwise keep.
func (t *CatalogTx) RebuildFactsFTS() error {
	_, err := t.q.Exec(fmt.Sprintf(`INSERT INTO %s(%s) VALUES('rebuild')`, FactsFTSTable, FactsFTSTable))
	return err
}

// ErrCompactionBusy reports that another connection kept the WAL from being
// checkpointed; compaction must be retried.
var ErrCompactionBusy = errors.New("compaction pending: another pudl process holds the catalog open")

// CompactScrubbed checkpoints the WAL, vacuums and checkpoints again on one
// connection, so free pages and WAL frames that held scrubbed values are
// gone, then marks scrubIDs compacted.
func (c *CatalogDB) CompactScrubbed(ctx context.Context, scrubIDs []string) error {
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	checkpoint := func() error {
		var busy, logFrames, checkpointed int
		if err := conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
			return err
		}
		if busy != 0 {
			return ErrCompactionBusy
		}
		return nil
	}
	if err := checkpoint(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "VACUUM"); err != nil {
		if strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked") {
			return ErrCompactionBusy
		}
		return fmt.Errorf("vacuum: %w", err)
	}
	if err := checkpoint(); err != nil {
		return err
	}
	now := formatCatalogTime(time.Now().UTC())
	for _, id := range scrubIDs {
		if _, err := conn.ExecContext(ctx, `UPDATE sensitive_scrubs SET compacted_at = ? WHERE scrub_id = ?`, now, id); err != nil {
			return err
		}
	}
	return nil
}

func (c *CatalogDB) queryStrings(query string, args ...any) ([]string, error) {
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (c *CatalogDB) hasTable(name string) (bool, error) {
	var exists bool
	err := c.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, name).Scan(&exists)
	return exists, err
}

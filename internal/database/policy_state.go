package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/errors"
)

// Compliance policy results (internal/policyfacts) are facts under the
// relations PolicyStatusRelation and PolicyViolationRelation, owned by the
// source PolicySource(policy, resource) and reconciled like projected facts.
// policy_state records, per resource and policy, which catalog entry and which
// policy digest the current facts describe.

// PolicySourcePrefix marks facts owned by policy evaluation. Other writers
// must not use it: the next evaluation would close their facts.
const PolicySourcePrefix = "policy:"

// PolicySource is the fact source owning one resource's results for one policy.
func PolicySource(policy, resourceID string) string {
	return PolicySourcePrefix + policy + ":" + resourceID
}

// IsPolicySource reports whether source is reserved for policy evaluation.
func IsPolicySource(source string) bool {
	return strings.HasPrefix(source, PolicySourcePrefix)
}

// PolicyState is one policy_state row.
type PolicyState struct {
	ResourceID  string `json:"resource_id"`
	Policy      string `json:"policy"`
	EntryID     string `json:"entry_id"`
	Type        string `json:"type"`
	Digest      string `json:"policy_digest"`
	Status      string `json:"status"`
	EvaluatedAt int64  `json:"evaluated_at"`
}

func (c *CatalogDB) ensurePolicyState() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS policy_state (
			resource_id  TEXT NOT NULL,
			policy       TEXT NOT NULL,
			entry_id     TEXT NOT NULL,
			type         TEXT NOT NULL,
			digest       TEXT NOT NULL,
			status       TEXT NOT NULL,
			evaluated_at INTEGER NOT NULL,
			PRIMARY KEY (resource_id, policy)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_policy_state_policy ON policy_state(policy)`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			return fmt.Errorf("create policy state: %w", err)
		}
	}
	return nil
}

const selectPolicyStateSQL = `SELECT resource_id, policy, entry_id, type, digest, status, evaluated_at FROM policy_state`

func listPolicyStateIn(q dbtx, where string, args ...any) ([]PolicyState, error) {
	rows, err := q.Query(selectPolicyStateSQL+" "+where+` ORDER BY resource_id, policy`, args...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "list policy state", err)
	}
	defer rows.Close()
	var out []PolicyState
	for rows.Next() {
		var s PolicyState
		if err := rows.Scan(&s.ResourceID, &s.Policy, &s.EntryID, &s.Type, &s.Digest, &s.Status, &s.EvaluatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PolicyStates returns a resource's rows.
func (t *CatalogTx) PolicyStates(resourceID string) ([]PolicyState, error) {
	return listPolicyStateIn(t.q, `WHERE resource_id = ?`, resourceID)
}

// PutPolicyState records which entry and digest a result's facts describe.
func (t *CatalogTx) PutPolicyState(s PolicyState) error {
	if s.EvaluatedAt == 0 {
		s.EvaluatedAt = time.Now().Unix()
	}
	_, err := t.q.Exec(`INSERT INTO policy_state (resource_id, policy, entry_id, type, digest, status, evaluated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(resource_id, policy) DO UPDATE SET entry_id = excluded.entry_id, type = excluded.type,
			digest = excluded.digest, status = excluded.status, evaluated_at = excluded.evaluated_at`,
		s.ResourceID, s.Policy, s.EntryID, s.Type, s.Digest, s.Status, s.EvaluatedAt)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "write policy state", err)
	}
	return nil
}

// DeletePolicyState forgets one result.
func (t *CatalogTx) DeletePolicyState(resourceID, policy string) error {
	_, err := t.q.Exec(`DELETE FROM policy_state WHERE resource_id = ? AND policy = ?`, resourceID, policy)
	return err
}

// ListPolicyState reads every row. A catalog opened read-only before the
// table existed has none.
func (c *CatalogDB) ListPolicyState() ([]PolicyState, error) {
	var n int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'policy_state'`).Scan(&n); err != nil || n == 0 {
		return nil, err
	}
	return listPolicyStateIn(c.db, "")
}

// ListPolicyState reads every row inside the transaction.
func (t *CatalogTx) ListPolicyState() ([]PolicyState, error) {
	return listPolicyStateIn(t.q, "")
}

// PolicyOrphans returns rows whose entry is gone or no longer has the row's
// resource ID (deleted, pruned, re-identified or reassigned).
func (t *CatalogTx) PolicyOrphans() ([]PolicyState, error) {
	return listPolicyStateIn(t.q, `WHERE NOT EXISTS (SELECT 1 FROM catalog_entries e
		WHERE e.id = policy_state.entry_id AND e.resource_id = policy_state.resource_id)`)
}

// ScopeLeavers returns the rows of resources last evaluated from an entry
// observed in an earlier complete observation of scope, that the complete
// snapshot snapshotID of the same scope does not contain.
func (t *CatalogTx) ScopeLeavers(scope, snapshotID string) ([]PolicyState, error) {
	return listPolicyStateIn(t.q, `WHERE EXISTS (SELECT 1 FROM collection_memberships m
			JOIN observe_snapshots s ON s.snapshot_id = m.collection_id
			WHERE m.item_id = policy_state.entry_id AND s.scope = ? AND s.complete = 1 AND s.snapshot_id != ?)
		AND resource_id NOT IN (SELECT e.resource_id FROM collection_memberships m
			JOIN catalog_entries e ON e.id = m.item_id
			WHERE m.collection_id = ? AND e.resource_id IS NOT NULL)`, scope, snapshotID, snapshotID)
}

// CurrentResourceEntries returns, for every resource with a resolved
// identity, the entry describing its current state: the entry its policy
// results or projected facts were last computed from, else its most recently
// imported entry. Observations that deduplicate to an older entry are
// therefore not mistaken for the newest import.
func (t *CatalogTx) CurrentResourceEntries() ([]CatalogEntry, error) {
	return currentResourceEntriesIn(t.q)
}

// CurrentResourceEntries is the read-only form.
func (c *CatalogDB) CurrentResourceEntries() ([]CatalogEntry, error) {
	return currentResourceEntriesIn(c.db)
}

func currentResourceEntriesIn(q dbtx) ([]CatalogEntry, error) {
	rows, err := q.Query(`SELECT ` + entrySelect("catalog_entries") + `,
			COALESCE((SELECT p.entry_id FROM policy_state p WHERE p.resource_id = catalog_entries.resource_id
				ORDER BY p.evaluated_at DESC LIMIT 1),
				(SELECT s.entry_id FROM projection_state s WHERE s.resource_id = catalog_entries.resource_id), '')
		FROM catalog_entries
		WHERE resource_id IS NOT NULL AND identity_json IS NOT NULL AND identity_json != ''
		ORDER BY resource_id, import_timestamp DESC, rowid DESC`)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "list current resources", err)
	}
	defer rows.Close()
	var out []CatalogEntry
	recorded := map[string]string{} // resource -> entry its results describe
	for rows.Next() {
		var cur string
		scanned, err := scanEntry(scanWithExtra{rows: rows, extra: &cur})
		if err != nil {
			return nil, err
		}
		entry := *scanned
		rid := *entry.ResourceID
		if len(out) == 0 || *out[len(out)-1].ResourceID != rid {
			out = append(out, entry)
			recorded[rid] = cur
			continue
		}
		// A later row of the same resource is the entry its results describe.
		if recorded[rid] == entry.ID {
			out[len(out)-1] = entry
		}
	}
	return out, rows.Err()
}

// scanWithExtra scans an entry row followed by one extra column.
type scanWithExtra struct {
	rows  rowScanner
	extra any
}

func (s scanWithExtra) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, s.extra)...)
}

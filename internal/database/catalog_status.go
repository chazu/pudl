package database

import (
	"fmt"
	"time"
)

// TargetStatus represents the convergence status of a single target.
type TargetStatus struct {
	Target    string
	Status    string
	UpdatedAt time.Time
	DiffCount int // from latest drift report, 0 if clean
}

// UpdateStatus sets the convergence status for entries matching a target name.
// Only updates the latest entry for the target.
func (c *CatalogDB) UpdateStatus(targetName string, status string) error {
	return updateStatusIn(c.db, targetName, status)
}

// updateStatusIn is the executor-parameterized form of UpdateStatus, so the
// per-action statuses of one apply commit with the manifest that produced them.
func updateStatusIn(q dbtx, targetName string, status string) error {
	validStatuses := map[string]bool{
		"unknown": true, "clean": true, "drifted": true,
		"converging": true, "failed": true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	_, err := q.Exec(
		`UPDATE catalog_entries SET status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE target = ? AND id = (
		     SELECT id FROM catalog_entries WHERE target = ?
		     ORDER BY import_timestamp DESC LIMIT 1
		 )`,
		status, targetName, targetName,
	)
	return err
}

// PromoteConvergingToClean flips status converging -> clean for the latest entry
// of each named target that is currently "converging", returning the number
// promoted. It is the drift re-check verifying a pending apply: when a model's
// drift is ∅, its resources that ingest-manifest left "applied, pending
// verification" (converging) are now confirmed in sync. Targets not currently
// converging (or absent) are untouched, so it is safe to call with a superset of
// candidate names.
//
// This is the fallback for manifests ingested without `--model`, where the rows
// carry no model tag and the only key available is the bare target name. Target
// names are not unique across models — two models each declaring a resource
// named `nginx` produce the same key — so promoting on the name alone let one
// model's clean drift promote another's pending rows. The model predicate keeps
// the promotion to rows this model could plausibly own: its own tagged rows, and
// untagged rows (the case this fallback exists for). Rows tagged to a *different*
// model are never promoted.
//
// Untagged rows from two different models sharing a target name remain
// indistinguishable — nothing in the row records which model applied it. Tag
// them by ingesting with `ingest-manifest --model <name>`, which routes the
// promotion through PromoteConvergingToCleanByModel and skips this path.
func (c *CatalogDB) PromoteConvergingToClean(targets []string, model string) (int, error) {
	promoted := 0
	for _, def := range targets {
		res, err := c.db.Exec(
			`UPDATE catalog_entries SET status = 'clean', updated_at = CURRENT_TIMESTAMP
			 WHERE target = ? AND status = 'converging'
			   AND (json_extract(tags, '$.model') IS NULL OR json_extract(tags, '$.model') = ?)
			   AND id = (
			     SELECT id FROM catalog_entries WHERE target = ?
			     ORDER BY import_timestamp DESC LIMIT 1
			 )`,
			def, model, def,
		)
		if err != nil {
			return promoted, fmt.Errorf("promote %q: %w", def, err)
		}
		n, _ := res.RowsAffected()
		promoted += int(n)
	}
	return promoted, nil
}

// PromoteConvergingToCleanByModel flips status converging -> clean for every entry
// tagged with the given model (`tags.model`, set by `ingest-manifest --model`). This
// is the exact form of the drift re-check verifying a pending apply: when the model's
// drift is ∅, all its resources that ingest-manifest left "converging" are confirmed
// in sync — without reconstructing the resource→model mapping from desired records.
// Returns the number promoted.
func (c *CatalogDB) PromoteConvergingToCleanByModel(model string) (int, error) {
	if model == "" {
		return 0, nil
	}
	res, err := c.db.Exec(
		`UPDATE catalog_entries SET status = 'clean', updated_at = CURRENT_TIMESTAMP
		 WHERE status = 'converging' AND json_extract(tags, '$.model') = ?`,
		model,
	)
	if err != nil {
		return 0, fmt.Errorf("promote converging for model %q: %w", model, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// GetTargetStatuses returns the latest status for each target that has entries.
func (c *CatalogDB) GetTargetStatuses() ([]TargetStatus, error) {
	return getTargetStatusesIn(c.db)
}

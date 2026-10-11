package projection

import (
	"context"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/redact"
)

// LoadFunc decodes a catalog entry's stored payload.
type LoadFunc func(entry database.CatalogEntry) (any, error)

// SyncReport is what a sync changed.
type SyncReport struct {
	Failures     map[string][]string `json:"failures,omitempty"`
	Orphaned     int                 `json:"orphaned"`     // sources closed: entry gone, re-identified or schema lost its facts
	Reprojected  int                 `json:"reprojected"`  // resources recomputed after a spec change
	Bootstrapped int                 `json:"bootstrapped"` // resources projected for the first time
	Broken       map[string]string   `json:"broken,omitempty"`
	Errors       []string            `json:"errors,omitempty"` // per-entry failures (payload unreadable); facts left as they were
}

// Changed reports whether the sync wrote anything.
func (r SyncReport) Changed() bool {
	return r.Orphaned+r.Reprojected+r.Bootstrapped > 0
}

// Sync brings projected facts in line with the catalog and the current specs,
// as corrections (retractions), in one transaction:
//
//  1. orphans — state whose entry was deleted, re-identified or reassigned, or
//     whose schema no longer projects: the source is emptied, the row dropped;
//  2. stale — state whose spec fingerprint changed: recomputed from its entry;
//  3. bootstrap — resources of projecting schemas with no state: the most
//     recently imported entry is projected.
//
// Schemas whose spec is broken are left alone (existing facts unchanged) and
// reported. With dryRun nothing is written; the report says what would be.
func Sync(ctx context.Context, db *database.CatalogDB, reg *Registry, load LoadFunc, dryRun bool) (SyncReport, error) {
	report := SyncReport{Broken: map[string]string{}}
	var schemas []string
	for _, schema := range reg.Schemas() {
		spec := reg.For(schema)
		if spec.Err != nil {
			report.Broken[schema] = spec.Err.Error()
			continue
		}
		schemas = append(schemas, schema)
	}
	err := db.WithCatalogTxContext(ctx, func(tx *database.CatalogTx) error {
		orphans, err := tx.ProjectionOrphans()
		if err != nil {
			return err
		}
		orphaned := map[string]bool{}
		for _, row := range orphans {
			orphaned[row.ResourceID] = true
			if err := retire(tx, row.ResourceID, dryRun); err != nil {
				return err
			}
			report.Orphaned++
		}

		states, err := tx.ListProjectionState()
		if err != nil {
			return err
		}
		for _, row := range states {
			if orphaned[row.ResourceID] {
				continue
			}
			spec := reg.For(row.Schema)
			if spec == nil {
				if err := retire(tx, row.ResourceID, dryRun); err != nil {
					return err
				}
				report.Orphaned++
				continue
			}
			if spec.Err != nil || spec.Fingerprint == row.Fingerprint {
				continue
			}
			entry, err := tx.GetEntry(row.EntryID)
			if err != nil {
				return err
			}
			if project(tx, reg, load, *entry, dryRun, &report) {
				report.Reprojected++
			}
		}

		pending, err := tx.EntriesNeedingProjection(schemas)
		if err != nil {
			return err
		}
		for _, entry := range pending {
			if project(tx, reg, load, entry, dryRun, &report) {
				report.Bootstrapped++
			}
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if err == errDryRun {
		err = nil
	}
	sort.Strings(report.Errors)
	return report, err
}

var errDryRun = fmt.Errorf("projection sync dry run")

// retire empties a resource's projected facts and forgets its state.
func retire(tx *database.CatalogTx, resourceID string, dryRun bool) error {
	if dryRun {
		return nil
	}
	if _, _, err := tx.ReconcileProjection(database.ProjectionSource(resourceID), nil, database.ProjectionCorrection); err != nil {
		return err
	}
	return tx.DeleteProjectionState(resourceID)
}

// project recomputes one entry's facts as a correction. A payload that cannot
// be read is reported and its facts left as they were.
func project(tx *database.CatalogTx, reg *Registry, load LoadFunc, entry database.CatalogEntry, dryRun bool, report *SyncReport) bool {
	spec := reg.For(entry.Schema)
	if spec == nil || spec.Err != nil || entry.ResourceID == nil {
		return false
	}
	data, err := load(entry)
	if err != nil {
		report.recordFailure(entry, err)
		return false
	}
	// Payloads stored before a field became sensitive are redacted here, so
	// the secret cannot reach facts.
	redact.Apply(data, spec.Sensitive)
	prepared, _ := Prepare(reg, entry.Schema, entry.ID, *entry.ResourceID, entry.IdentityJSON != nil, data)
	if dryRun {
		return true
	}
	if err := apply(tx, prepared, database.ProjectionCorrection); err != nil {
		report.recordFailure(entry, err)
		return false
	}
	return true
}

func (r *SyncReport) recordFailure(entry database.CatalogEntry, err error) {
	message := fmt.Sprintf("%s: %v", entry.ID, err)
	r.Errors = append(r.Errors, message)
	if r.Failures == nil {
		r.Failures = map[string][]string{}
	}
	r.Failures[entry.Schema] = append(r.Failures[entry.Schema], message)
}

// Stale reports, without writing, why a Sync would change anything: reasons
// are empty when projected facts are current.
func Stale(db *database.CatalogDB, reg *Registry) ([]string, error) {
	var reasons []string
	var schemas []string
	for _, schema := range reg.Schemas() {
		spec := reg.For(schema)
		if spec.Err != nil {
			reasons = append(reasons, fmt.Sprintf("projection disabled for %s: %v", schema, spec.Err))
			continue
		}
		schemas = append(schemas, schema)
	}
	orphans, err := db.ProjectionOrphans()
	if err != nil {
		return nil, err
	}
	if len(orphans) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d resources' projected facts describe entries that were deleted or re-identified", len(orphans)))
	}
	states, err := db.ListProjectionState()
	if err != nil {
		return nil, err
	}
	stale := 0
	for _, row := range states {
		spec := reg.For(row.Schema)
		if spec == nil || (spec.Err == nil && spec.Fingerprint != row.Fingerprint) {
			stale++
		}
	}
	if stale > 0 {
		reasons = append(reasons, fmt.Sprintf("%d resources were projected with an older facts spec", stale))
	}
	pending, err := db.EntriesNeedingProjection(schemas)
	if err != nil {
		return nil, err
	}
	if len(pending) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d resources have never been projected", len(pending)))
	}
	return reasons, nil
}

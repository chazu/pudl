package inventory

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

// Drift computes drift for an inventory model: desired against the observed
// records in scope (set-diff by identity), with each changed field's value in
// the model's previous successful observation.
//
// The scope is required. Without one the query returns every observation in
// the catalog — every model, every host, all time — so a desired record could
// be satisfied by an unrelated model's observation and the check could report
// clean against records it has nothing to do with. Callers pass either the
// snapshot they just populated or an explicitly requested replay scope.
//
// Verified is deliberately left false here: Drift cannot tell whether its
// scope names a fresh snapshot or a stale replay, so the caller that chose the
// scope is the one that gets to claim verification.
func Drift(ctx context.Context, db *database.CatalogDB, scope string, desired []map[string]any, policy Policy) (acute.ModelDriftResult, error) {
	if strings.TrimSpace(scope) == "" {
		return acute.ModelDriftResult{}, fmt.Errorf("inventory drift requires a catalog scope (snapshot ID or origin); refusing to compare against every observation in the catalog")
	}
	observed, err := LoadObserved(ctx, db, scope)
	if err != nil {
		return acute.ModelDriftResult{}, err
	}
	history := acute.PreviousInventory{Status: "no-baseline"}
	if observed.SnapshotID != "" {
		previous, pruned, err := db.PreviousSuccessfulObserveSnapshot(ctx, observed.SnapshotID)
		if err != nil {
			return acute.ModelDriftResult{}, err
		}
		if previous != nil {
			at := previous.CreatedAt
			history.SnapshotID = previous.SnapshotID
			history.ObservedAt = &at
			if pruned {
				history.Status = "pruned"
			} else {
				prior, err := LoadObserved(ctx, db, previous.SnapshotID)
				if err != nil {
					return acute.ModelDriftResult{}, fmt.Errorf("load previous observation: %w", err)
				}
				history.Status = "available"
				history.Records = prior.Records
			}
		}
	}
	identity := policy.Identity
	originalLabels := map[string]string{}
	if policy.Namespace != nil {
		normalized := make([]map[string]any, len(desired))
		for i, record := range desired {
			normalized[i], err = NormalizeRecord(record, policy.Namespace)
			if err == nil {
				_, canonicalLabel, _ := acute.RecordIdentity(normalized[i], identity)
				_, displayLabel, _ := acute.RecordIdentity(record, identity)
				originalLabels[canonicalLabel] = displayLabel
			}
			if err != nil {
				return acute.ModelDriftResult{}, err
			}
		}
		desired = normalized
		for i := range observed.Records {
			observed.Records[i].Data, err = NormalizeRecord(observed.Records[i].Data, policy.Namespace)
			if err != nil {
				return acute.ModelDriftResult{}, err
			}
		}
		for i := range history.Records {
			history.Records[i].Data, err = NormalizeRecord(history.Records[i].Data, policy.Namespace)
			if err != nil {
				return acute.ModelDriftResult{}, err
			}
		}
	}
	drifted := acute.InventorySetDiffWithPrevious(desired, observed.Records, identity, history)
	for i := range drifted {
		if label, ok := originalLabels[drifted[i].Resource]; ok {
			drifted[i].Resource = label
		}
	}
	uncertain := false
	if !observed.Complete {
		for i := range drifted {
			if drifted[i].Reason == acute.DriftMissing {
				drifted[i].Reason = "not-observed"
				drifted[i].Diff = "not observed in a partial inventory; absence is not established"
				uncertain = true
			}
		}
	}
	return acute.ModelDriftResult{
		Uncertain:  uncertain,
		Clean:      len(drifted) == 0,
		Drifted:    drifted,
		SnapshotID: observed.SnapshotID,
		ObservedAt: observed.ObservedAt,

		IdentityFallbacks: acute.IdentityFallbacks(identity, desired, observed.Records),
	}, nil
}

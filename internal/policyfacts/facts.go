package policyfacts

import (
	"encoding/json"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policy"
)

// Facts renders one result as facts:
//
//	policy_status(resource, policy, type, status, policy_digest)
//	policy_violation(resource, policy, rule, severity, path, kind, declared_by)
//
// Absences that are not violations make the status unknown and are not
// violation facts. No fact carries a record value.
func Facts(resourceID, entryID string, r *policy.Result) []database.Fact {
	provenance := mustJSON(map[string]string{"entry_id": entryID, "type": r.Type})
	facts := []database.Fact{{
		Relation: database.PolicyStatusRelation,
		Args: mustJSON(map[string]string{
			"resource": resourceID, "policy": r.Policy, "type": r.Type,
			"status": string(r.Status), "policy_digest": r.Digest,
		}),
		Provenance: provenance,
	}}
	for _, v := range r.Violations {
		facts = append(facts, database.Fact{
			Relation: database.PolicyViolationRelation,
			Args: mustJSON(map[string]string{
				"resource": resourceID, "policy": r.Policy, "rule": v.Rule,
				"severity": string(v.Severity), "path": v.Path, "kind": string(v.Kind),
				"declared_by": v.DeclaredBy,
			}),
			Provenance: provenance,
		})
	}
	return facts
}

func mustJSON(v map[string]string) string {
	b, _ := json.Marshal(v) // a map of strings always encodes
	return string(b)
}

// Change counts what an Observe wrote.
type Change struct {
	// Updated counts results whose facts changed (new results included).
	Updated int `json:"updated"`
	// Closed counts results closed: the policy no longer applies, or is no
	// longer loaded.
	Closed int `json:"closed"`
}

func (c *Change) add(o Change) {
	c.Updated += o.Updated
	c.Closed += o.Closed
}

// Observe makes p the resource's current results, in the transaction that
// records the ingest. mode says why replaced facts close: an observation
// (database.ProjectionObservation) or a correction. Results of covered
// policies that no longer apply are closed.
func Observe(tx *database.CatalogTx, p *Prepared, mode database.ProjectionMode) (Change, error) {
	var change Change
	if p == nil {
		return change, nil
	}
	states, err := tx.PolicyStates(p.ResourceID)
	if err != nil {
		return change, err
	}
	stale := map[string]bool{}
	for _, s := range states {
		stale[s.Policy] = p.covers(s.Policy)
	}
	for _, r := range p.Results {
		delete(stale, r.Policy)
		added, closed, err := tx.ReconcileProjection(database.PolicySource(r.Policy, p.ResourceID), Facts(p.ResourceID, p.EntryID, r), mode)
		if err != nil {
			return change, err
		}
		if added+closed > 0 {
			change.Updated++
		}
		if err := tx.PutPolicyState(database.PolicyState{
			ResourceID: p.ResourceID, Policy: r.Policy, EntryID: p.EntryID,
			Type: r.Type, Digest: r.Digest, Status: string(r.Status),
		}); err != nil {
			return change, err
		}
	}
	for name, closing := range stale {
		if closing {
			if err := closeResult(tx, p.ResourceID, name, mode); err != nil {
				return change, err
			}
			change.Closed++
		}
	}
	return change, nil
}

// closeResult closes one result's facts and forgets its state.
func closeResult(tx *database.CatalogTx, resourceID, policyName string, mode database.ProjectionMode) error {
	if _, _, err := tx.ReconcileProjection(database.PolicySource(policyName, resourceID), nil, mode); err != nil {
		return err
	}
	return tx.DeletePolicyState(resourceID, policyName)
}

// CloseScopeLeavers closes the results of resources that an earlier complete
// observation of scope contained and the complete snapshot snapshotID does
// not: they left the observed population, so their results are invalidated.
// It must run in the transaction that records the snapshot.
func CloseScopeLeavers(tx *database.CatalogTx, scope, snapshotID string) (int, error) {
	leavers, err := tx.ScopeLeavers(scope, snapshotID)
	if err != nil {
		return 0, err
	}
	for _, s := range leavers {
		if err := closeResult(tx, s.ResourceID, s.Policy, database.ProjectionObservation); err != nil {
			return 0, err
		}
	}
	return len(leavers), nil
}

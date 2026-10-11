package projection

import (
	"github.com/chazu/pudl/internal/database"
)

// Prepared is one record's projection, computed before the catalog commit so
// no decode or projection work happens under the write lock.
type Prepared struct {
	Incomplete  bool            `json:"incomplete,omitempty"`
	Schema      string          `json:"schema"`
	Fingerprint string          `json:"fingerprint"`
	ResourceID  string          `json:"resource_id"`
	EntryID     string          `json:"entry_id"`
	Resolved    bool            `json:"resolved"` // identity extracted
	Facts       []database.Fact `json:"facts,omitempty"`
}

// Prepare computes a record's projection, or nil when its schema projects
// nothing or its spec is broken (which callers report; existing facts are
// left alone so a typo cannot silently empty a check).
func Prepare(reg *Registry, schema, entryID, resourceID string, identityResolved bool, data any) (*Prepared, Result) {
	spec := reg.For(schema)
	if spec == nil || spec.Err != nil {
		return nil, Result{}
	}
	p := &Prepared{Schema: spec.Schema, Fingerprint: spec.Fingerprint, ResourceID: resourceID, EntryID: entryID, Resolved: identityResolved}
	if !identityResolved {
		return p, Result{}
	}
	res := Compute(spec, entryID, resourceID, data)
	p.Facts = res.Facts
	p.Incomplete = len(res.Omitted) > 0
	return p, res
}

// Observe records that p's entry is the latest observation of its resource:
// the resource's projected facts become p's (closing the previous
// observation's by invalidation). Re-observing the entry already current is a
// no-op, so dedup hits are cheap. It must run in the transaction that records
// the observation.
func Observe(tx *database.CatalogTx, p *Prepared) error {
	return apply(tx, p, database.ProjectionObservation)
}

// apply makes p the resource's current projection, closing what it replaces
// according to mode.
func apply(tx *database.CatalogTx, p *Prepared, mode database.ProjectionMode) error {
	if p == nil {
		return nil
	}
	status := database.ProjectionProjected
	if p.Incomplete {
		status = database.ProjectionIncomplete
	}
	if !p.Resolved {
		// Content-hash identity: every state of the resource would be its own
		// source and facts from all of them would stay current. Record the
		// skip so Sync does not retry it.
		status = database.ProjectionSkipped
	}
	state, err := tx.GetProjectionState(p.ResourceID)
	if err != nil {
		return err
	}
	if state != nil && state.EntryID == p.EntryID && state.Fingerprint == p.Fingerprint && state.Status == status {
		return nil
	}
	if _, _, err := tx.ReconcileProjection(database.ProjectionSource(p.ResourceID), p.Facts, mode); err != nil {
		return err
	}
	return tx.PutProjectionState(database.ProjectionState{
		ResourceID: p.ResourceID, EntryID: p.EntryID, Schema: p.Schema,
		Fingerprint: p.Fingerprint, Status: status,
	})
}

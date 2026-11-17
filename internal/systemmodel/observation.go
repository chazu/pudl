package systemmodel

import (
	"fmt"
	"strings"
)

// Observation declares the population a successful collection covers.
//
// A Scope holding ItemPlaceholder is per input: each input `each` enumerates
// is its own observation, with the placeholder replaced by the input (see
// docs/design/2026-11-16-per-input-scope.md).
type Observation struct {
	Scope    string   `json:"scope,omitempty"`
	Complete bool     `json:"complete,omitempty"`
	Schemas  []string `json:"schemas,omitempty"`
}

// PerInput reports whether the scope names one observation per enumerated
// input.
func (o *Observation) PerInput() bool {
	return o != nil && strings.Contains(o.Scope, ItemPlaceholder)
}

// ScopeFor is the scope of one input's observation: the placeholder
// replaced by item. A fixed scope is returned unchanged.
func (o Observation) ScopeFor(item string) string {
	return strings.ReplaceAll(o.Scope, ItemPlaceholder, item)
}

// validateObservation rejects a per-input scope the populate cannot give an
// item to: one without `each`, one whose fallback would answer with other
// inputs, and a model whose desired state is compared with one snapshot.
func (m *SystemModel) validateObservation() error {
	if !m.Observation.PerInput() {
		return nil
	}
	p := m.Populate
	switch {
	case p.Each == nil && (p.HTTP == nil || p.HTTP.Each == nil):
		return fmt.Errorf("observation scope %q uses %s but the populate declares no `each`", m.Observation.Scope, ItemPlaceholder)
	case p.Fallback != nil:
		return fmt.Errorf("observation scope %q uses %s, which a populate with a fallback cannot use: the fallback's inputs are not the populate's", m.Observation.Scope, ItemPlaceholder)
	case len(m.Desired) > 0:
		return fmt.Errorf("observation scope %q uses %s, which a model with desired cannot use: desired is compared with one snapshot", m.Observation.Scope, ItemPlaceholder)
	}
	return nil
}

// Package policyfacts persists compliance policy results (internal/policy) as
// bitemporal facts.
//
// Each (resource, policy) result is the fact source
// database.PolicySource(policy, resource) holding one policy_status fact and a
// policy_violation fact per violation. The source is reconciled on every
// evaluation, the same way schema projection reconciles its sources, so
// re-evaluation replaces current facts and keeps history. policy_state records
// which entry and policy digest each result describes.
//
// See docs/design/2026-11-01-compliance-policies.md.
package policyfacts

import (
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policy"
	"github.com/chazu/pudl/internal/schemaname"
)

// LoadFunc decodes a catalog entry's stored payload.
type LoadFunc func(entry database.CatalogEntry) (any, error)

// Evaluator evaluates the loaded policies against records.
type Evaluator struct {
	set        *policy.Set
	loadErrors []policy.LoadError
	// broken are policies that failed to load: their existing results are
	// left as they are, so a typo cannot silently empty them.
	broken map[string]bool
}

// New loads every policy of src. A nil src yields an evaluator with no
// policies.
func New(src policy.Source) *Evaluator {
	e := &Evaluator{broken: map[string]bool{}}
	if src == nil {
		return e
	}
	e.set, e.loadErrors = policy.Load(src)
	for _, le := range e.loadErrors {
		e.broken[schemaname.Normalize(le.Policy)] = true
	}
	return e
}

// Set returns the loaded policies, or nil when there are none.
func (e *Evaluator) Set() *policy.Set {
	if e == nil {
		return nil
	}
	return e.set
}

// LoadErrors reports the policies that failed to load.
func (e *Evaluator) LoadErrors() []policy.LoadError {
	if e == nil {
		return nil
	}
	return e.loadErrors
}

// Applicable returns the policies that apply to typeName.
func (e *Evaluator) Applicable(typeName string) []*policy.Policy {
	if e == nil || e.set == nil {
		return nil
	}
	return e.set.Applicable(typeName)
}

// Prepared is one record's policy results, computed before the catalog
// commit so no evaluation happens under the write lock.
type Prepared struct {
	ResourceID string           `json:"resource_id"`
	EntryID    string           `json:"entry_id"`
	Type       string           `json:"type"`
	Results    []*policy.Result `json:"results,omitempty"`
	// Scope names the policies this evaluation covered; empty means every
	// loaded policy. Results of covered policies that no longer apply are
	// closed.
	Scope []string `json:"scope,omitempty"`
	// Kept names policies whose existing results stay as they are: the
	// policy failed to load, or failed to evaluate this record.
	Kept []string `json:"kept,omitempty"`
	// Errors explain evaluation failures.
	Errors []string `json:"errors,omitempty"`
}

// Prepare evaluates every applicable policy against an ingested record. It
// returns nil when the record's identity is unresolved: with content-hash
// identity every state would be its own resource and every old result would
// stay current, as with projection.
func (e *Evaluator) Prepare(typeName, entryID, resourceID string, resolved bool, record any) *Prepared {
	if e == nil || !resolved {
		return nil
	}
	return e.prepare(e.Applicable(typeName), nil, typeName, entryID, resourceID, record)
}

// prepare evaluates the given policies; scope names what the evaluation
// covers (nil for everything).
func (e *Evaluator) prepare(policies []*policy.Policy, scope []string, typeName, entryID, resourceID string, record any) *Prepared {
	p := &Prepared{ResourceID: resourceID, EntryID: entryID, Type: schemaname.Normalize(typeName), Scope: scope}
	for name := range e.broken {
		p.Kept = append(p.Kept, name)
	}
	if len(policies) == 0 {
		sort.Strings(p.Kept)
		return p
	}
	data, ok := record.(map[string]any)
	for _, pol := range policies {
		var res *policy.Result
		err := fmt.Errorf("record is not an object")
		if ok {
			res, err = e.set.Evaluate(pol, typeName, data)
		}
		if err != nil {
			p.Kept = append(p.Kept, pol.Name)
			p.Errors = append(p.Errors, fmt.Sprintf("%s: policy %s: %v", entryID, pol.Name, err))
			continue
		}
		p.Results = append(p.Results, res)
	}
	sort.Strings(p.Kept)
	return p
}

// covers reports whether the evaluation decides policy's result.
func (p *Prepared) covers(policyName string) bool {
	for _, k := range p.Kept {
		if k == policyName {
			return false
		}
	}
	if len(p.Scope) == 0 {
		return true
	}
	for _, s := range p.Scope {
		if s == policyName {
			return true
		}
	}
	return false
}

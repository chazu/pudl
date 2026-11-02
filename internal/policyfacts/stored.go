package policyfacts

import (
	"encoding/json"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policy"
	"github.com/chazu/pudl/internal/validator"
)

// Stored is one stored result: its policy_state row and its current
// policy_violation facts.
type Stored struct {
	database.PolicyState
	// HighestSeverity is the most severe violation's, derived when read.
	HighestSeverity policy.Severity    `json:"highest_severity,omitempty"`
	Violations      []policy.Violation `json:"violations"`
}

// ReadStored returns the stored results of resourceID and policyName ("" matches
// any), in policy_state order, with their current violations.
func ReadStored(db *database.CatalogDB, resourceID, policyName string) ([]Stored, error) {
	states, err := db.ListPolicyState(resourceID, policyName)
	if err != nil || len(states) == 0 {
		return []Stored{}, err
	}
	violations, err := db.PolicyViolations(resourceID, policyName)
	if err != nil {
		return nil, err
	}
	out := make([]Stored, len(states))
	index := map[string]int{}
	for i, s := range states {
		out[i] = Stored{PolicyState: s, Violations: []policy.Violation{}}
		index[s.ResourceID+"\x00"+s.Policy] = i
	}
	for _, args := range violations {
		var a struct {
			Resource, Policy, Rule, Severity, Path, Kind string
			DeclaredBy                                   string `json:"declared_by"`
		}
		if json.Unmarshal([]byte(args), &a) != nil {
			continue
		}
		if i, ok := index[a.Resource+"\x00"+a.Policy]; ok {
			out[i].Violations = append(out[i].Violations, policy.Violation{
				Rule: a.Rule, Severity: policy.Severity(a.Severity), Path: a.Path,
				Kind: policy.Kind(a.Kind), DeclaredBy: a.DeclaredBy,
			})
		}
	}
	for i := range out {
		policy.SortViolations(out[i].Violations)
		out[i].HighestSeverity = (&policy.Result{Violations: out[i].Violations}).HighestSeverity()
	}
	return out, nil
}

// Review returns a type schema's review; draft when no policy is loaded.
func (e *Evaluator) Review(typeName string) validator.SchemaReview {
	if e.set == nil {
		return validator.SchemaReview{Status: validator.ReviewDraft}
	}
	return e.set.Review(typeName)
}

// IsStale reports whether a stored result was evaluated with a digest other
// than its loaded policy's.
func (e *Evaluator) IsStale(s database.PolicyState) bool {
	p, loaded := e.lookup(s.Policy)
	return loaded && p.Digest != s.Digest
}

// Summary describes one loaded policy and counts its stored results.
type Summary struct {
	PolicySummary
	Extends string `json:"extends,omitempty"`
	Rules   int    `json:"rules"`
	// Review is the review status of the policy's type: draft or validated.
	Review string `json:"review"`
	// Stale counts results evaluated with an older version of the policy.
	Stale int `json:"stale"`
	// LastEvaluated is the latest evaluation (Unix seconds), 0 when never.
	LastEvaluated int64 `json:"last_evaluated,omitempty"`
}

// Summarize describes each policy with the counts of its stored results. A
// policy never evaluated has zero counts.
func (e *Evaluator) Summarize(states []database.PolicyState, policies []*policy.Policy) []Summary {
	out := make([]Summary, len(policies))
	byName := map[string]*Summary{}
	for i, p := range policies {
		out[i] = Summary{PolicySummary: PolicySummary{Policy: p.Name, Type: p.Type, Digest: p.Digest},
			Extends: p.Extends, Rules: len(p.Rules), Review: e.Review(p.Type).Status}
		byName[p.Name] = &out[i]
	}
	for _, s := range states {
		sum := byName[s.Policy]
		if sum == nil {
			continue
		}
		sum.count(policy.Status(s.Status), e.Review(s.Type).Status != validator.ReviewValidated)
		if e.IsStale(s) {
			sum.Stale++
		}
		sum.LastEvaluated = max(sum.LastEvaluated, s.EvaluatedAt)
	}
	return out
}

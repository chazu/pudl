package policyfacts

import (
	"sort"

	"github.com/chazu/pudl/internal/policy"
)

// maxTopViolations bounds Report.TopViolations.
const maxTopViolations = 10

// Report is what an evaluation over stored resources found and changed.
type Report struct {
	// Resources counts resources at least one selected policy applied to.
	Resources int             `json:"resources"`
	Policies  []PolicySummary `json:"policies"`
	// TopViolations are the rules failing on most resources, most severe
	// first.
	TopViolations []RuleSummary `json:"top_violations,omitempty"`
	// Change counts results written (or, in a dry run, that would be);
	// Orphaned counts results closed because their entry was deleted or
	// re-identified. Both stay zero for a what-if evaluation.
	Change
	Orphaned int `json:"orphaned"`
	// Errors are per-record evaluation failures; their results were left as
	// they were. LoadErrors are policies that failed to load.
	Errors     []string `json:"errors,omitempty"`
	LoadErrors []string `json:"load_errors,omitempty"`
}

// PolicySummary counts one policy's results by status.
type PolicySummary struct {
	Policy       string `json:"policy"`
	Type         string `json:"type"`
	Digest       string `json:"policy_digest"`
	Compliant    int    `json:"compliant"`
	Noncompliant int    `json:"noncompliant"`
	Unknown      int    `json:"unknown"`
	// Provisional counts results against a type not reviewed as validated.
	Provisional int `json:"provisional"`
}

// RuleSummary counts the resources one rule fails on.
type RuleSummary struct {
	Policy    string          `json:"policy"`
	Rule      string          `json:"rule"`
	Severity  policy.Severity `json:"severity"`
	Kind      policy.Kind     `json:"kind"`
	Resources int             `json:"resources"`
}

// tally accumulates results into a Report.
type tally struct {
	report   Report
	policies map[string]*PolicySummary
	rules    map[RuleSummary]int
}

func newTally(policies []*policy.Policy) *tally {
	t := &tally{policies: map[string]*PolicySummary{}, rules: map[RuleSummary]int{}}
	for _, p := range policies {
		t.policies[p.Name] = &PolicySummary{Policy: p.Name, Type: p.Type, Digest: p.Digest}
	}
	return t
}

func (t *tally) note(p *Prepared) {
	if p == nil {
		return
	}
	t.report.Errors = append(t.report.Errors, p.Errors...)
	if len(p.Results) > 0 {
		t.report.Resources++
	}
	for _, r := range p.Results {
		s := t.policies[r.Policy]
		if s == nil {
			continue
		}
		switch r.Status {
		case policy.StatusCompliant:
			s.Compliant++
		case policy.StatusNoncompliant:
			s.Noncompliant++
		case policy.StatusUnknown:
			s.Unknown++
		}
		if r.Provisional {
			s.Provisional++
		}
		seen := map[RuleSummary]bool{}
		for _, v := range r.Violations {
			key := RuleSummary{Policy: r.Policy, Rule: v.Rule, Severity: v.Severity, Kind: v.Kind}
			if !seen[key] {
				seen[key] = true
				t.rules[key]++
			}
		}
	}
}

func (t *tally) finish(loadErrors []policy.LoadError) Report {
	r := t.report
	r.Policies = []PolicySummary{}
	for _, s := range t.policies {
		r.Policies = append(r.Policies, *s)
	}
	sort.Slice(r.Policies, func(i, j int) bool { return r.Policies[i].Policy < r.Policies[j].Policy })
	for key, n := range t.rules {
		key.Resources = n
		r.TopViolations = append(r.TopViolations, key)
	}
	sort.Slice(r.TopViolations, func(i, j int) bool {
		a, b := r.TopViolations[i], r.TopViolations[j]
		if a.Severity.Rank() != b.Severity.Rank() {
			return a.Severity.Rank() > b.Severity.Rank()
		}
		if a.Resources != b.Resources {
			return a.Resources > b.Resources
		}
		return a.Policy+"\x00"+a.Rule+"\x00"+string(a.Kind) < b.Policy+"\x00"+b.Rule+"\x00"+string(b.Kind)
	})
	if len(r.TopViolations) > maxTopViolations {
		r.TopViolations = r.TopViolations[:maxTopViolations]
	}
	for _, le := range loadErrors {
		r.LoadErrors = append(r.LoadErrors, le.Error())
	}
	sort.Strings(r.Errors)
	return r
}

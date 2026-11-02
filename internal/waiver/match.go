package waiver

import (
	"time"

	"github.com/chazu/pudl/internal/schemaname"
)

// State is what became of a waiver in one check.
type State string

const (
	// Active: the waiver applies and matched at least one finding.
	Active State = "active"
	// Expired: the waiver's expires date has passed. Its findings count
	// again; Matches says how many it would still cover.
	Expired State = "expired"
	// Stale: the waiver applies but matched no finding.
	Stale State = "stale"
)

// Ref identifies the waiver a finding matched: its position in the model's
// waivers list and what an operator needs to judge it.
type Ref struct {
	Index   int    `json:"index"`
	Owner   string `json:"owner"`
	Reason  string `json:"reason"`
	Expires string `json:"expires"`
	// State is active when the waiver excuses the finding, expired when it
	// would have but has expired (the finding counts).
	State State `json:"state"`
}

// Outcome is one declared waiver after a check: its state and how many
// findings it matched.
type Outcome struct {
	Waiver
	Index   int   `json:"index"`
	State   State `json:"state"`
	Matches int   `json:"matches"`
}

// Violation is one current policy violation, as a policy waiver sees it.
type Violation struct {
	Rule, Severity, Path, Kind string
}

// ViolationLookup returns the current violations of resource under policy.
type ViolationLookup func(resource, policy string) ([]Violation, error)

// Matcher decides, finding by finding, which of a model's waivers apply, and
// counts what each matched. It is not safe for concurrent use.
type Matcher struct {
	waivers []Waiver
	expired []bool
	matches []int
	lookup  ViolationLookup
	cache   map[[2]string][]Violation
}

// NewMatcher prepares waivers as of now. lookup may be nil when no waiver
// targets a policy violation.
func NewMatcher(waivers []Waiver, now time.Time, lookup ViolationLookup) *Matcher {
	m := &Matcher{waivers: waivers, expired: make([]bool, len(waivers)), matches: make([]int, len(waivers)), lookup: lookup, cache: map[[2]string][]Violation{}}
	for i, w := range waivers {
		m.expired[i] = w.Expired(now)
	}
	return m
}

// Match decides whether a finding — a witness of check with arguments args —
// is waived. It returns the active waiver that excuses it; else an expired
// waiver that would have, so the report can flag it; else nil.
//
// A check waiver matches a witness of its check that carries every selector
// value. A policy waiver matches the violations a witness stands for: a
// witness with resource and policy arguments stands for that resource's
// current violations of the policy, narrowed by any rule, severity, path or
// kind argument it also carries. Such a witness is waived only when it stands
// for at least one violation and every one has an active waiver.
func (m *Matcher) Match(check string, args map[string]any) (*Ref, error) {
	if m == nil || len(m.waivers) == 0 {
		return nil, nil
	}
	if ref := m.matchCheck(check, args); ref != nil {
		return ref, nil
	}
	return m.matchPolicy(args)
}

func (m *Matcher) matchCheck(check string, args map[string]any) *Ref {
	var expired *Ref
	for i, w := range m.waivers {
		if w.IsPolicy() || w.Check != check || !selects(w.Witness, args) {
			continue
		}
		m.matches[i]++
		if !m.expired[i] {
			return m.ref(i)
		}
		if expired == nil {
			expired = m.ref(i)
		}
	}
	return expired
}

func (m *Matcher) matchPolicy(args map[string]any) (*Ref, error) {
	resource, _ := args["resource"].(string)
	policyName, _ := args["policy"].(string)
	if resource == "" || policyName == "" || !m.anyPolicyWaiver(resource, policyName) {
		return nil, nil
	}
	violations, err := m.violations(resource, policyName)
	if err != nil {
		return nil, err
	}
	var active, expired *Ref
	uncovered := false
	for _, v := range violations {
		if !narrows(args, v) {
			continue
		}
		i, ok := m.policyWaiver(resource, policyName, v.Rule)
		if !ok {
			uncovered = true
			continue
		}
		m.matches[i]++
		switch {
		case m.expired[i] && expired == nil:
			expired = m.ref(i)
		case !m.expired[i] && active == nil:
			active = m.ref(i)
		}
	}
	if uncovered || expired != nil || active == nil {
		return expired, nil
	}
	return active, nil
}

// policyWaiver returns the waiver for one violation, preferring an active
// one.
func (m *Matcher) policyWaiver(resource, policyName, rule string) (int, bool) {
	found := -1
	for i, w := range m.waivers {
		if !w.IsPolicy() || w.Resource != resource || w.Rule != rule || !samePolicy(w.Policy, policyName) {
			continue
		}
		if !m.expired[i] {
			return i, true
		}
		if found < 0 {
			found = i
		}
	}
	return found, found >= 0
}

func (m *Matcher) anyPolicyWaiver(resource, policyName string) bool {
	for _, w := range m.waivers {
		if w.IsPolicy() && w.Resource == resource && samePolicy(w.Policy, policyName) {
			return true
		}
	}
	return false
}

func (m *Matcher) violations(resource, policyName string) ([]Violation, error) {
	key := [2]string{resource, policyName}
	if v, ok := m.cache[key]; ok {
		return v, nil
	}
	if m.lookup == nil {
		return nil, nil
	}
	v, err := m.lookup(resource, policyName)
	if err != nil {
		return nil, err
	}
	m.cache[key] = v
	return v, nil
}

func (m *Matcher) ref(i int) *Ref {
	w := m.waivers[i]
	state := Active
	if m.expired[i] {
		state = Expired
	}
	return &Ref{Index: i, Owner: w.Owner, Reason: w.Reason, Expires: w.Expires, State: state}
}

// Outcomes reports every declared waiver, in declaration order: expired,
// active (it matched) or stale (it matched nothing).
func (m *Matcher) Outcomes() []Outcome {
	if m == nil || len(m.waivers) == 0 {
		return nil
	}
	out := make([]Outcome, len(m.waivers))
	for i, w := range m.waivers {
		state := Active
		switch {
		case m.expired[i]:
			state = Expired
		case m.matches[i] == 0:
			state = Stale
		}
		out[i] = Outcome{Waiver: w, Index: i, State: state, Matches: m.matches[i]}
	}
	return out
}

// selects reports whether args carry every selector value.
func selects(selector, args map[string]any) bool {
	for k, want := range selector {
		got, ok := args[k]
		if !ok || canonical(got) != canonical(want) {
			return false
		}
	}
	return true
}

// narrows reports whether a witness that carries rule, severity, path or kind
// arguments stands for violation v.
func narrows(args map[string]any, v Violation) bool {
	for k, value := range map[string]string{"rule": v.Rule, "severity": v.Severity, "path": v.Path, "kind": v.Kind} {
		if got, ok := args[k]; ok && canonical(got) != canonical(value) {
			return false
		}
	}
	return true
}

func samePolicy(a, b string) bool {
	return a == b || schemaname.Normalize(a) == schemaname.Normalize(b)
}

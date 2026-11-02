// Package waiver declares and matches check waivers: accepted, time-boxed
// exceptions to a model's check findings. A waiver is declared on the
// #SystemModel next to the checks it excuses, and targets either one check's
// witnesses (check + witness selector) or one compliance-policy violation
// (policy + rule + resource). Every waiver names an owner, a reason, the day
// it was created and the day it expires.
//
// The package is pure: it validates declarations and decides, per finding,
// which waiver applies. Policy violations are looked up through an injected
// function, so it has no catalog dependency.
package waiver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DateLayout is the format of created and expires: a calendar date.
const DateLayout = "2006-01-02"

// Waiver is one declared exception, decoded from a model's `waivers` list.
type Waiver struct {
	// Check and Witness target a check's findings: the check's name in the
	// model, and witness argument values a finding must carry (all of them).
	Check   string         `json:"check,omitempty"`
	Witness map[string]any `json:"witness,omitempty"`
	// Policy, Rule and Resource target one compliance-policy violation: the
	// policy, the rule it violates and the violating resource's ID.
	Policy   string `json:"policy,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Resource string `json:"resource,omitempty"`

	Owner  string `json:"owner"`
	Reason string `json:"reason"`
	// Created and Expires are YYYY-MM-DD dates. A waiver applies through its
	// expires date (UTC) and stops applying the next day.
	Created string `json:"created"`
	Expires string `json:"expires"`
}

// IsPolicy reports whether the waiver targets a policy violation rather than
// a check's witnesses.
func (w Waiver) IsPolicy() bool { return w.Policy != "" }

// Target describes what the waiver excuses, for human output.
func (w Waiver) Target() string {
	if w.IsPolicy() {
		return fmt.Sprintf("policy %s rule %s resource %s", w.Policy, w.Rule, w.Resource)
	}
	keys := make([]string, 0, len(w.Witness))
	for k := range w.Witness {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + canonical(w.Witness[k])
	}
	return fmt.Sprintf("check %s witness %s", w.Check, strings.Join(parts, " "))
}

// Expired reports whether now is after the waiver's expires date (UTC). An
// unparseable date counts as expired; Validate rejects one at load.
func (w Waiver) Expired(now time.Time) bool {
	expires, err := time.Parse(DateLayout, w.Expires)
	if err != nil {
		return true
	}
	return !now.UTC().Before(expires.AddDate(0, 0, 1))
}

// Validate checks a model's waivers strictly. checks maps each of the model's
// check names to its expect ("empty" or "nonempty"); a check waiver must name
// an `expect: "empty"` check, since only those have witnesses.
func Validate(waivers []Waiver, checks map[string]string) error {
	for i, w := range waivers {
		if err := validateOne(w, checks); err != nil {
			return fmt.Errorf("waivers[%d]: %w", i, err)
		}
	}
	return nil
}

func validateOne(w Waiver, checks map[string]string) error {
	for _, f := range []struct{ name, value string }{{"owner", w.Owner}, {"reason", w.Reason}, {"created", w.Created}, {"expires", w.Expires}} {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("%s is required", f.name)
		}
	}
	created, err := time.Parse(DateLayout, w.Created)
	if err != nil {
		return fmt.Errorf("created %q is not a YYYY-MM-DD date", w.Created)
	}
	expires, err := time.Parse(DateLayout, w.Expires)
	if err != nil {
		return fmt.Errorf("expires %q is not a YYYY-MM-DD date", w.Expires)
	}
	if expires.Before(created) {
		return fmt.Errorf("expires %s is before created %s", w.Expires, w.Created)
	}
	hasCheck := w.Check != "" || len(w.Witness) > 0
	hasPolicy := w.Policy != "" || w.Rule != "" || w.Resource != ""
	switch {
	case hasCheck && hasPolicy:
		return fmt.Errorf("target either a check (check, witness) or a policy violation (policy, rule, resource), not both")
	case hasPolicy:
		if w.Policy == "" || w.Rule == "" || w.Resource == "" {
			return fmt.Errorf("a policy waiver requires policy, rule and resource")
		}
	case hasCheck:
		if w.Check == "" || len(w.Witness) == 0 {
			return fmt.Errorf("a check waiver requires check and a non-empty witness selector")
		}
		expect, ok := checks[w.Check]
		if !ok {
			return fmt.Errorf("check %q is not a check of this model", w.Check)
		}
		if expect != "empty" {
			return fmt.Errorf("check %q expects %s; only expect: \"empty\" checks have witnesses to waive", w.Check, expect)
		}
	default:
		return fmt.Errorf("a waiver requires a target: check and witness, or policy, rule and resource")
	}
	return nil
}

// canonical encodes a value for comparison, so a CUE int and a Datalog int64
// or float64 of the same number compare equal.
func canonical(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

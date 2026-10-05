package datalog

import (
	"fmt"
	"sort"
	"strings"
)

// checkRule validates a single rule that was read successfully: its shape and
// range restriction (every head variable must be bound by a body atom). It is
// applied at load time so a broken rule is reported where it is written, not
// when a query first happens to reach it.
func checkRule(r Rule) error {
	if r.Head.Rel == "" {
		return fmt.Errorf("head has an empty rel")
	}
	if len(r.Head.Args) == 0 {
		return fmt.Errorf("head has no arguments")
	}
	if len(r.Body) == 0 {
		return fmt.Errorf("empty body")
	}

	bound := make(map[string]bool)
	for i, atom := range r.Body {
		if atom.Rel == "" {
			return fmt.Errorf("body atom %d has an empty rel", i)
		}
		for _, key := range sortedArgKeys(atom.Args) {
			term := atom.Args[key]
			if term.IsAggregate() {
				return fmt.Errorf("body atom %d (%s): aggregate %s() is only allowed in a rule head", i, atom.Rel, term.Agg)
			}
			if term.IsVariable() {
				bound[term.Variable] = true
			}
		}
	}

	for _, key := range sortedArgKeys(r.Head.Args) {
		term := r.Head.Args[key]
		if term.IsComparison() {
			return fmt.Errorf("head argument %s: comparison %s is only allowed in a rule body", key, term.Cmp)
		}
		if term.IsVariable() && !bound[term.Variable] {
			return fmt.Errorf("head argument %s: variable %s is not bound in the body", key, term.Variable)
		}
	}
	return nil
}

// InvalidRules returns the rules that failed to load, in their original order.
func InvalidRules(rules []Rule) []Rule {
	var out []Rule
	for _, r := range rules {
		if !r.Valid() {
			out = append(out, r)
		}
	}
	return out
}

// RuleSetProblems describes every problem in a loaded rule set, one line each:
// rules that failed to load, and relations whose rules disagree on head keys.
// It is empty for a healthy rule set.
func RuleSetProblems(rules []Rule) []string {
	var problems []string
	for _, r := range InvalidRules(rules) {
		problems = append(problems, fmt.Sprintf("%s: %v", r.Describe(), r.LoadErr))
	}
	mismatches := headKeyProblems(validRules(rules))
	for _, rel := range sortedRelations(mismatches) {
		problems = append(problems, mismatches[rel].Error())
	}
	return problems
}

// headKeyProblems reports each relation whose rules do not all project the same
// head key set. A relation's columns are its head keys, so rules that disagree
// would union rows of different shapes.
func headKeyProblems(rules []Rule) map[string]error {
	first := make(map[string]Rule)
	problems := make(map[string]error)
	for _, r := range rules {
		f, seen := first[r.Head.Rel]
		if !seen {
			first[r.Head.Rel] = r
			continue
		}
		if _, done := problems[r.Head.Rel]; done {
			continue
		}
		want, got := sortedArgKeys(f.Head.Args), sortedArgKeys(r.Head.Args)
		if strings.Join(want, "\x00") != strings.Join(got, "\x00") {
			problems[r.Head.Rel] = fmt.Errorf("relation %q: rule %s has head keys (%s) but rule %s has (%s); every rule of a relation must use the same keys",
				r.Head.Rel, f.Describe(), strings.Join(want, ", "), r.Describe(), strings.Join(got, ", "))
		}
	}
	return problems
}

func sortedRelations(m map[string]error) []string {
	out := make([]string, 0, len(m))
	for rel := range m {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

// validRules returns the rules that loaded without error.
func validRules(rules []Rule) []Rule {
	out := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.Valid() {
			out = append(out, r)
		}
	}
	return out
}

// dependencyClosure returns relation plus every relation it transitively reads
// through the bodies of the given rules.
func dependencyClosure(rules []Rule, relation string) map[string]bool {
	byHead := make(map[string][]Rule)
	for _, r := range rules {
		byHead[r.Head.Rel] = append(byHead[r.Head.Rel], r)
	}
	closure := map[string]bool{relation: true}
	queue := []string{relation}
	for len(queue) > 0 {
		rel := queue[0]
		queue = queue[1:]
		for _, r := range byHead[rel] {
			for _, atom := range r.Body {
				if !closure[atom.Rel] {
					closure[atom.Rel] = true
					queue = append(queue, atom.Rel)
				}
			}
		}
	}
	return closure
}

// rulesForQuery returns the valid rules, or an error when a query on relation
// depends on a problem: an invalid rule in its dependency closure, or a
// relation in the closure whose rules disagree on head keys. An invalid rule
// whose head could not be read might derive anything, so it blocks every query.
func rulesForQuery(rules []Rule, relation string) ([]Rule, error) {
	valid := validRules(rules)
	closure := dependencyClosure(valid, relation)

	var blocking []Rule
	for _, r := range InvalidRules(rules) {
		if r.Head.Rel == "" || closure[r.Head.Rel] {
			blocking = append(blocking, r)
		}
	}
	if len(blocking) > 0 {
		return nil, invalidRulesError(relation, blocking)
	}

	mismatches := headKeyProblems(valid)
	for _, rel := range sortedRelations(mismatches) {
		if closure[rel] {
			return nil, mismatches[rel]
		}
	}
	return valid, nil
}

func invalidRulesError(relation string, rules []Rule) error {
	lines := make([]string, 0, len(rules))
	for _, r := range rules {
		lines = append(lines, fmt.Sprintf("  %s: %v", r.Describe(), r.LoadErr))
	}
	sort.Strings(lines)
	return fmt.Errorf("relation %q depends on %d invalid rule(s); fix them or run `pudl doctor`:\n%s",
		relation, len(rules), strings.Join(lines, "\n"))
}

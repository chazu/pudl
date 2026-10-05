// Package eval provides public access to pudl's Datalog rule types and loaders.
// This is the external API for consumers; the query execution path lives on
// factstore.Store.Query.
package eval

import (
	"github.com/chazu/pudl/internal/datalog"
)

// Types re-exported for external consumers. All are plain data structures, so
// the aliases are usable without importing internal packages.
type (
	Rule  = datalog.Rule
	Atom  = datalog.Atom
	Term  = datalog.Term
	Tuple = datalog.Tuple
)

// Var creates a variable term.
func Var(name string) Term { return datalog.Var(name) }

// Val creates a ground value term.
func Val(v interface{}) Term { return datalog.Val(v) }

// LoadRulesFromPaths loads rules from CUE files in the given directories.
//
// A malformed rule is returned with Rule.LoadErr set rather than dropped; a
// query whose dependency closure contains one fails with an error naming it.
// Use InvalidRules to report them up front.
func LoadRulesFromPaths(paths ...string) ([]Rule, error) {
	return datalog.LoadRulesFromPaths(paths...)
}

// ParseRulesFromSource parses rules from a CUE source string. Numeric ground
// terms retain json.Number values; query compilation checks their numeric range.
// Malformed rules are returned with Rule.LoadErr set, as for LoadRulesFromPaths.
func ParseRulesFromSource(source string) ([]Rule, error) {
	return datalog.ParseRulesFromSource(source)
}

// InvalidRules returns the rules that failed to load.
func InvalidRules(rules []Rule) []Rule {
	return datalog.InvalidRules(rules)
}

// RuleSetProblems describes every problem in a rule set, one line each.
func RuleSetProblems(rules []Rule) []string {
	return datalog.RuleSetProblems(rules)
}

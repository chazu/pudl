// Package eval provides public access to pudl's Datalog rule types and loaders.
// This is the external API for consumers; the query execution path lives on
// factstore.Store.Query.
package eval

import (
	"pudl/internal/datalog"
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
func LoadRulesFromPaths(paths ...string) ([]Rule, error) {
	return datalog.LoadRulesFromPaths(paths...)
}

// ParseRulesFromSource parses rules from a CUE source string.
func ParseRulesFromSource(source string) ([]Rule, error) {
	return datalog.ParseRulesFromSource(source)
}

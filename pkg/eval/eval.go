// Package eval provides public access to pudl's Datalog evaluator.
// This is the external API for consumers like nous.
package eval

import (
	"pudl/internal/database"
	"pudl/internal/datalog"
)

// Types re-exported for external consumers.
type (
	Rule    = datalog.Rule
	Atom    = datalog.Atom
	Term    = datalog.Term
	Tuple   = datalog.Tuple
	EDB     = datalog.EDB
)

// Var creates a variable term.
func Var(name string) Term { return datalog.Var(name) }

// Val creates a ground value term.
func Val(v interface{}) Term { return datalog.Val(v) }

// NewEvaluator creates a Datalog evaluator.
func NewEvaluator(rules []Rule, edb EDB) *datalog.Evaluator {
	return datalog.NewEvaluator(rules, edb)
}

// NewMultiEDB combines multiple EDB sources.
func NewMultiEDB(sources ...EDB) *datalog.MultiEDB {
	return datalog.NewMultiEDB(sources...)
}

// NewFactsEDB creates an EDB backed by the facts table.
func NewFactsEDB(db *database.CatalogDB) *datalog.FactsEDB {
	return datalog.NewFactsEDB(db)
}

// NewCatalogEDB creates an EDB backed by the catalog_entries table.
func NewCatalogEDB(db *database.CatalogDB) *datalog.CatalogEDB {
	return datalog.NewCatalogEDB(db)
}

// LoadRulesFromPaths loads rules from CUE files in the given directories.
func LoadRulesFromPaths(paths ...string) ([]Rule, error) {
	return datalog.LoadRulesFromPaths(paths...)
}

// ParseRulesFromSource parses rules from a CUE source string.
func ParseRulesFromSource(source string) ([]Rule, error) {
	return datalog.ParseRulesFromSource(source)
}

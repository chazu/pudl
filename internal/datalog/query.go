package datalog

import (
	"fmt"

	"pudl/internal/database"
)

// Evaluate runs the full Datalog query path for a single relation:
// partition rules into recursive/non-recursive, evaluate non-recursive rules
// (and base EDB facts) via SQL, then fall back to the recursive fixpoint
// evaluator when recursive rules exist and the SQL pass produced nothing.
//
// This is the single source of truth shared by the CLI (`pudl query`) and the
// public API (pkg/factstore).
func Evaluate(db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	recursive, nonRecursive := PartitionRules(rules)

	// If the queried relation is derived by a recursive rule, the fixpoint
	// evaluator is authoritative: it seeds the base (non-recursive) rules and
	// computes the full closure. The SQL path alone would return only the base
	// tuples and miss the recursive expansion.
	if relationHasRecursiveRule(relation, recursive) {
		results, err := EvalRecursive(db, rules, relation, constraints, scope)
		if err != nil {
			return nil, fmt.Errorf("recursive query failed: %w", err)
		}
		return results, nil
	}

	// Non-recursive relation (or a base EDB relation): evaluate via SQL.
	sqlEval := NewSQLEvaluator(db, nonRecursive, scope)
	results, err := sqlEval.Query(relation, constraints)
	if err != nil {
		return nil, fmt.Errorf("sql query failed: %w", err)
	}

	// Safety net: a non-recursive relation may transitively depend on a
	// recursive relation that the SQL compiler cannot expand. If SQL found
	// nothing and recursive rules exist, retry through the fixpoint evaluator.
	if len(results) == 0 && len(recursive) > 0 {
		results, err = EvalRecursive(db, rules, relation, constraints, scope)
		if err != nil {
			return nil, fmt.Errorf("recursive query failed: %w", err)
		}
	}

	return results, nil
}

// relationHasRecursiveRule reports whether the given relation is the head of any
// recursive rule.
func relationHasRecursiveRule(relation string, recursive []Rule) bool {
	for _, r := range recursive {
		if r.Head.Rel == relation {
			return true
		}
	}
	return false
}

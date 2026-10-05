package datalog

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/database"
)

// DefaultMaxIterations is the default cap on semi-naive rounds for one cyclic
// component. Each round extends a recursive derivation by one step, so the cap
// bounds the longest chain a recursive relation can follow.
const DefaultMaxIterations = 100

// EvalOptions tunes evaluation. The zero value uses the defaults.
type EvalOptions struct {
	// MaxIterations caps semi-naive rounds per cyclic component;
	// 0 means DefaultMaxIterations.
	MaxIterations int
}

func (o EvalOptions) maxIterations() int {
	if o.MaxIterations > 0 {
		return o.MaxIterations
	}
	return DefaultMaxIterations
}

// Evaluate answers a query for a single relation with the default options.
//
// This is the single source of truth shared by the CLI (`pudl query`), model
// checks, and the public API (pkg/factstore).
func Evaluate(db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	return EvaluateWithOptions(db, rules, relation, constraints, scope, EvalOptions{})
}

// EvaluateWithOptions answers a query for a single relation. It plans only the
// relation's dependency closure, so rules the query never reads are neither
// checked for cycles nor evaluated:
//
//   - a relation no rule produces is read from stored facts;
//   - a closure without cycles compiles to one SQL statement, each derived
//     relation a CTE in dependency order (aggregates allowed anywhere);
//   - a closure with cycles is materialized stratum by stratum, iterating only
//     the cyclic components to a fixpoint.
func EvaluateWithOptions(db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope, opts EvalOptions) ([]Tuple, error) {
	return EvaluateContext(context.Background(), db, rules, relation, constraints, scope, opts)
}

// EvaluateContext evaluates with cancellable SQL and recursive rounds.
func EvaluateContext(ctx context.Context, db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope, opts EvalOptions) (tuples []Tuple, resultErr error) {
	defer func() {
		if resultErr != nil && ctx.Err() != nil {
			tuples = nil
			resultErr = ctx.Err()
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Built-in EDB relations (e.g. catalog_entry) are join-only: they resolve
	// inside rule bodies but cannot be queried directly. Querying one with no
	// producing rule would silently fall through to the facts table and return
	// nothing, so fail loudly instead.
	if _, isBuiltin := builtinEDBTables[relation]; isBuiltin && !relationHasAnyRule(relation, rules) {
		return nil, fmt.Errorf("relation %q is a join-only built-in (catalog) relation: reference it in a rule body, or list catalog entries directly instead of querying it", relation)
	}

	rules, err := rulesForQuery(rules, relation)
	if err != nil {
		return nil, err
	}
	constraints, err = database.QueryConstraints(constraints)
	if err != nil {
		return nil, err
	}
	plan, err := planQuery(rules, relation)
	if err != nil {
		return nil, err
	}

	switch {
	case !plan.derived():
		return evalEDBContext(ctx, db, relation, constraints, scope)
	case plan.hasCycle():
		results, err := evalStratifiedContext(ctx, db, plan, constraints, scope, opts.maxIterations())
		if err != nil {
			return nil, fmt.Errorf("recursive query failed: %w", err)
		}
		return results, nil
	default:
		results, err := evalAcyclicContext(ctx, db, plan, constraints, scope)
		if err != nil {
			return nil, fmt.Errorf("sql query failed: %w", err)
		}
		return results, nil
	}
}

// relationHasAnyRule reports whether the given relation is the head of any rule.
func relationHasAnyRule(relation string, rules []Rule) bool {
	for _, r := range rules {
		if r.Head.Rel == relation {
			return true
		}
	}
	return false
}

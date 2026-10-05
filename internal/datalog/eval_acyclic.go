package datalog

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

// evalAcyclic answers a plan with no cycles in one SQL statement. Each derived
// relation in the closure becomes a common table expression, in dependency
// order, and a rule reading an earlier relation joins its CTE through
// TableOverrides — the same mechanism that maps catalog_entry to its view.
// Aggregates compose freely: a CTE that aggregates is just another table to
// the rules above it.
func evalAcyclic(db *database.CatalogDB, plan *queryPlan, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	overrides := withBuiltinEDB(nil)
	var ctes []string
	var params []interface{}
	for _, c := range plan.components {
		rel := c.rels[0]
		body, p, err := unionSQL(plan.byHead[rel], scope, overrides)
		if err != nil {
			return nil, err
		}
		ctes = append(ctes, fmt.Sprintf("%s AS (\n%s\n)", cteName(rel), body))
		params = append(params, p...)
		overrides[rel] = cteName(rel)
	}

	query := "WITH " + strings.Join(ctes, ",\n") + "\nSELECT * FROM " + cteName(plan.relation) + " AS derived"
	query, params = whereConstraints(query, "derived.", constraints, params)

	rows, err := db.DB().Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("sql query: %w", err)
	}
	defer rows.Close()
	return scanTuples(rows, plan.relation)
}

// unionSQL compiles every rule of one relation and joins them with UNION. A
// relation is a set, so a tuple derived by two rules is one row; every rule
// projects the relation's shared head keys in sorted order, so the positional
// union aligns columns by name.
func unionSQL(rules []Rule, scope TemporalScope, overrides map[string]string) (string, []interface{}, error) {
	var parts []string
	var params []interface{}
	for _, rule := range rules {
		cq, err := CompileWithOptions(rule, scope, CompileOptions{TableOverrides: overrides})
		if err != nil {
			return "", nil, fmt.Errorf("compile rule %s: %w", rule.Name, err)
		}
		parts = append(parts, cq.SQL)
		params = append(params, cq.Params...)
	}
	return strings.Join(parts, "\nUNION\n"), params, nil
}

// cteName is the quoted CTE name for a derived relation.
func cteName(rel string) string {
	return quoteIdent("_cte_" + rel)
}

// whereConstraints appends equality filters on result columns, in sorted key
// order so the SQL text and its parameters are deterministic.
func whereConstraints(query, qualifier string, constraints map[string]interface{}, params []interface{}) (string, []interface{}) {
	keys := make([]string, 0, len(constraints))
	for k := range constraints {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var predicates []string
	for _, k := range keys {
		predicates = append(predicates, qualifier+quoteIdent(k)+" = ?")
		params = append(params, constraints[k])
	}
	if len(predicates) > 0 {
		query += " WHERE " + strings.Join(predicates, " AND ")
	}
	return query, params
}

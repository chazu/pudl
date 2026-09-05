package datalog

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

type SQLEvaluator struct {
	db    *database.CatalogDB
	rules []Rule
	scope TemporalScope
}

func NewSQLEvaluator(db *database.CatalogDB, rules []Rule, scope TemporalScope) *SQLEvaluator {
	return &SQLEvaluator{db: db, rules: rules, scope: scope}
}

func (e *SQLEvaluator) Query(relation string, constraints map[string]interface{}) ([]Tuple, error) {
	constraints, err := database.QueryConstraints(constraints)
	if err != nil {
		return nil, err
	}
	matching := e.rulesForRelation(relation)
	if len(matching) == 0 {
		return e.fallbackEDB(relation, constraints)
	}

	var queries []string
	var allParams []interface{}

	opts := CompileOptions{TableOverrides: builtinEDBTables}
	for _, rule := range matching {
		cq, err := CompileWithOptions(rule, e.scope, opts)
		if err != nil {
			return nil, fmt.Errorf("compile rule %s: %w", rule.Name, err)
		}
		queries = append(queries, cq.SQL)
		allParams = append(allParams, cq.Params...)
	}

	fullSQL := strings.Join(queries, "\nUNION ALL\n")

	if len(constraints) > 0 {
		fullSQL = fmt.Sprintf("SELECT * FROM (\n%s\n) AS derived", fullSQL)
		keys := make([]string, 0, len(constraints))
		for key := range constraints {
			keys = append(keys, key)
		}
		sortStrings(keys)
		var predicates []string
		for _, key := range keys {
			predicates = append(predicates, fmt.Sprintf("derived.\"%s\" = ?", strings.ReplaceAll(key, `"`, `""`)))
			allParams = append(allParams, constraints[key])
		}
		fullSQL += " WHERE " + strings.Join(predicates, " AND ")
	}

	rows, err := e.db.DB().Query(fullSQL, allParams...)
	if err != nil {
		return nil, fmt.Errorf("sql query: %w", err)
	}
	defer rows.Close()

	headKeys := e.headKeysForRelation(matching)
	return scanTuples(rows, relation, headKeys)
}

func (e *SQLEvaluator) rulesForRelation(relation string) []Rule {
	var result []Rule
	for _, r := range e.rules {
		if r.Head.Rel == relation {
			result = append(result, r)
		}
	}
	return result
}

func (e *SQLEvaluator) headKeysForRelation(rules []Rule) []string {
	if len(rules) == 0 {
		return nil
	}
	head := rules[0].Head
	keys := make([]string, 0, len(head.Args))
	for k, t := range head.Args {
		if t.IsVariable() {
			keys = append(keys, k)
		}
	}
	sortStrings(keys)
	return keys
}

func (e *SQLEvaluator) fallbackEDB(relation string, constraints map[string]interface{}) ([]Tuple, error) {
	var facts []database.Fact
	var err error

	if e.scope.ValidAt == nil && e.scope.TxAt == nil {
		if len(constraints) > 0 {
			facts, err = e.db.QueryCurrentFactsFiltered(relation, constraints)
		} else {
			facts, err = e.db.QueryCurrentFacts(relation)
		}
	} else {
		facts, err = e.db.QueryFacts(database.FactFilter{
			Relation: relation,
			ValidAt:  e.scope.ValidAt,
			TxAt:     e.scope.TxAt,
		})
	}
	if err != nil {
		return nil, err
	}

	tuples := make([]Tuple, 0, len(facts))
	for _, f := range facts {
		value, err := database.DecodeQueryJSON(f.Args)
		if err != nil {
			return nil, fmt.Errorf("fact %s: %w", f.ID, err)
		}
		args, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		t := Tuple{Relation: relation, Args: args}
		if matchConstraints(t, constraints) {
			tuples = append(tuples, t)
		}
	}
	return tuples, nil
}

func scanTuples(rows *sql.Rows, relation string, headKeys []string) ([]Tuple, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var tuples []Tuple
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		args := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			args[col] = normalizeValue(vals[i])
		}
		tuples = append(tuples, Tuple{Relation: relation, Args: args})
	}
	return tuples, rows.Err()
}

func normalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case []byte:
		return string(val)
	default:
		return val
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

package datalog

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chazu/pudl/internal/database"
)

// evalEDB answers a query on a relation no rule produces: its stored facts.
func evalEDBContext(ctx context.Context, db *database.CatalogDB, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	var facts []database.Fact
	var err error

	if scope.ValidAt == nil && scope.TxAt == nil {
		if len(constraints) > 0 {
			facts, err = db.QueryCurrentFactsFilteredContext(ctx, relation, constraints)
		} else {
			facts, err = db.QueryCurrentFactsContext(ctx, relation)
		}
	} else {
		facts, err = db.QueryFactsContext(ctx, database.FactFilter{
			Relation: relation,
			ValidAt:  scope.ValidAt,
			TxAt:     scope.TxAt,
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

// scanTuples reads result rows into tuples, one argument per column.
func scanTuples(rows *sql.Rows, relation string) ([]Tuple, error) {
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

package datalog

import (
	"fmt"
	"sort"
	"strings"
)

type TemporalScope struct {
	ValidAt *int64
	TxAt    *int64
}

type CompiledQuery struct {
	SQL    string
	Params []interface{}
	Head   Atom
	Vars   map[string]string // variable name → SQL expression (e.g. "json_extract(t0.args, '$.from')")
}

type CompileOptions struct {
	TableOverrides map[string]string
}

func Compile(rule Rule, scope TemporalScope) (*CompiledQuery, error) {
	return CompileWithOptions(rule, scope, CompileOptions{})
}

func CompileWithOptions(rule Rule, scope TemporalScope, opts CompileOptions) (*CompiledQuery, error) {
	if len(rule.Body) == 0 {
		return nil, fmt.Errorf("rule %s has no body atoms", rule.Name)
	}

	tableName := "current_facts"
	if scope.ValidAt != nil || scope.TxAt != nil {
		tableName = "facts"
	}

	varExprs := make(map[string]string)

	var fromParts []string
	var whereParts []string
	var params []interface{}

	for i, atom := range rule.Body {
		alias := fmt.Sprintf("t%d", i)

		override, hasOverride := opts.TableOverrides[atom.Rel]
		if hasOverride {
			fromParts = append(fromParts, fmt.Sprintf("%s %s", override, alias))
		} else {
			fromParts = append(fromParts, fmt.Sprintf("%s %s", tableName, alias))
			whereParts = append(whereParts, fmt.Sprintf("%s.relation = ?", alias))
			params = append(params, atom.Rel)
		}

		keys := sortedArgKeys(atom.Args)
		for _, key := range keys {
			term := atom.Args[key]
			if term.IsAggregate() {
				return nil, fmt.Errorf("rule %s: aggregate %s() not allowed in rule body", rule.Name, term.Agg)
			}
			var expr string
			if hasOverride {
				expr = fmt.Sprintf("%s.\"%s\"", alias, key)
			} else {
				expr = fmt.Sprintf("json_extract(%s.args, '$.%s')", alias, key)
			}

			if term.IsVariable() {
				if prev, seen := varExprs[term.Variable]; seen {
					whereParts = append(whereParts, fmt.Sprintf("%s = %s", prev, expr))
				} else {
					varExprs[term.Variable] = expr
				}
			} else {
				whereParts = append(whereParts, fmt.Sprintf("%s = ?", expr))
				params = append(params, term.Value)
			}
		}

		if !hasOverride {
			if scope.ValidAt != nil {
				whereParts = append(whereParts,
					fmt.Sprintf("%s.valid_start <= ?", alias),
					fmt.Sprintf("(%s.valid_end IS NULL OR %s.valid_end > ?)", alias, alias),
				)
				params = append(params, *scope.ValidAt, *scope.ValidAt)
			}

			if scope.TxAt != nil {
				whereParts = append(whereParts,
					fmt.Sprintf("%s.tx_start <= ?", alias),
					fmt.Sprintf("(%s.tx_end IS NULL OR %s.tx_end > ?)", alias, alias),
				)
				params = append(params, *scope.TxAt, *scope.TxAt)
			} else if scope.ValidAt != nil {
				whereParts = append(whereParts, fmt.Sprintf("%s.tx_end IS NULL", alias))
			}
		}
	}

	var selectParts []string
	var groupParts []string
	hasAgg := false
	headKeys := sortedArgKeys(rule.Head.Args)
	for _, key := range headKeys {
		term := rule.Head.Args[key]
		if !term.IsVariable() {
			continue
		}
		expr, ok := varExprs[term.Variable]
		if !ok {
			return nil, fmt.Errorf("head variable %s not bound in body", term.Variable)
		}
		if term.IsAggregate() {
			hasAgg = true
			selectParts = append(selectParts, fmt.Sprintf("%s(%s) AS \"%s\"", strings.ToUpper(term.Agg), expr, key))
		} else {
			selectParts = append(selectParts, fmt.Sprintf("%s AS \"%s\"", expr, key))
			groupParts = append(groupParts, expr)
		}
	}

	if len(selectParts) == 0 {
		return nil, fmt.Errorf("rule %s head has no variable projections", rule.Name)
	}

	var sql string
	if hasAgg {
		// Aggregated rule: non-aggregate head vars become GROUP BY keys; the
		// aggregate functions reduce within each group. No DISTINCT (the GROUP BY
		// already collapses rows). A head with only aggregates and no group keys
		// reduces over the whole relation (single row, no GROUP BY clause).
		sql = fmt.Sprintf("SELECT\n    %s\nFROM %s\nWHERE %s",
			strings.Join(selectParts, ",\n    "),
			strings.Join(fromParts, ", "),
			strings.Join(whereParts, "\n  AND "),
		)
		if len(groupParts) > 0 {
			sql += "\nGROUP BY " + strings.Join(groupParts, ", ")
		}
	} else {
		sql = fmt.Sprintf("SELECT DISTINCT\n    %s\nFROM %s\nWHERE %s",
			strings.Join(selectParts, ",\n    "),
			strings.Join(fromParts, ", "),
			strings.Join(whereParts, "\n  AND "),
		)
	}

	return &CompiledQuery{
		SQL:    sql,
		Params: params,
		Head:   rule.Head,
		Vars:   varExprs,
	}, nil
}

func sortedArgKeys(args map[string]Term) []string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

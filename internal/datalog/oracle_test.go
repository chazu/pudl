package datalog

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

// This file freezes the evaluator as it stood before stratified, goal-directed
// evaluation (after the head-semantics fixes), as a differential-test oracle.
// It routes any relation whose rules read a derived relation through one
// whole-rule-set fixpoint, and everything else through a SQL union. It is
// deliberately simple and slow; only its answers matter.

const oracleMaxIterations = 1000

func oracleEvaluate(db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	rules, err := rulesForQuery(rules, relation)
	if err != nil {
		return nil, err
	}
	constraints, err = database.QueryConstraints(constraints)
	if err != nil {
		return nil, err
	}
	recursive, nonRecursive := oraclePartition(rules)
	if oracleHasHead(relation, recursive) {
		return oracleFixpoint(db, rules, recursive, nonRecursive, relation, constraints, scope)
	}
	var matching []Rule
	for _, r := range nonRecursive {
		if r.Head.Rel == relation {
			matching = append(matching, r)
		}
	}
	if len(matching) == 0 {
		return oracleEDB(db, relation, constraints, scope)
	}
	var queries []string
	var params []interface{}
	for _, r := range matching {
		cq, err := CompileWithOptions(r, scope, CompileOptions{TableOverrides: builtinEDBTables})
		if err != nil {
			return nil, err
		}
		queries = append(queries, cq.SQL)
		params = append(params, cq.Params...)
	}
	query := "SELECT * FROM (\n" + strings.Join(queries, "\nUNION\n") + "\n) AS derived"
	query, params = oracleWhere(query, "derived.", constraints, params)
	rows, err := db.DB().Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTuples(rows, relation)
}

func oraclePartition(rules []Rule) (recursive, nonRecursive []Rule) {
	heads := derivedRelations(rules)
	for _, r := range rules {
		isRec := false
		for _, a := range r.Body {
			if heads[a.Rel] {
				isRec = true
			}
		}
		if isRec {
			recursive = append(recursive, r)
		} else {
			nonRecursive = append(nonRecursive, r)
		}
	}
	return
}

func oracleHasHead(relation string, rules []Rule) bool {
	for _, r := range rules {
		if r.Head.Rel == relation {
			return true
		}
	}
	return false
}

func oracleFixpoint(db *database.CatalogDB, all, recRules, baseRules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	derived := derivedRelations(all)
	cols := headColumnsOf(all)
	tx, err := db.DB().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for rel := range derived {
		for _, p := range []string{"_o_rule_", "_o_delta_", "_o_new_"} {
			c := colDefList(cols[rel])
			if _, err := tx.Exec(fmt.Sprintf("CREATE TEMP TABLE %s (%s, PRIMARY KEY(%s))", quoteIdent(p+rel), c, c)); err != nil {
				return nil, err
			}
		}
	}
	for _, r := range baseRules {
		cq, err := CompileWithOptions(r, scope, CompileOptions{TableOverrides: builtinEDBTables})
		if err != nil {
			return nil, err
		}
		for _, p := range []string{"_o_rule_", "_o_delta_"} {
			if _, err := tx.Exec(fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) %s", quoteIdent(p+r.Head.Rel), colDefList(cols[r.Head.Rel]), cq.SQL), cq.Params...); err != nil {
				return nil, err
			}
		}
	}

	overrides := map[string]string{}
	for rel := range derived {
		overrides[rel] = quoteIdent("_o_rule_" + rel)
	}
	overrides = withBuiltinEDB(overrides)
	var queries []*CompiledQuery
	for _, r := range recRules {
		for i, a := range r.Body {
			if !derived[a.Rel] {
				continue
			}
			cq, err := CompileWithOptions(r, scope, CompileOptions{TableOverrides: overrides, AtomTableOverrides: map[int]string{i: quoteIdent("_o_delta_" + a.Rel)}})
			if err != nil {
				return nil, err
			}
			queries = append(queries, cq)
		}
	}

	for iter := 0; ; iter++ {
		if iter == oracleMaxIterations {
			return nil, fmt.Errorf("oracle: fixpoint not reached")
		}
		for rel := range derived {
			if _, err := tx.Exec("DELETE FROM " + quoteIdent("_o_new_"+rel)); err != nil {
				return nil, err
			}
		}
		for _, cq := range queries {
			rel := cq.Head.Rel
			if _, err := tx.Exec(fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) %s", quoteIdent("_o_new_"+rel), colDefList(cols[rel]), cq.SQL), cq.Params...); err != nil {
				return nil, err
			}
		}
		var total int64
		for rel := range derived {
			c := colDefList(cols[rel])
			if _, err := tx.Exec("DELETE FROM " + quoteIdent("_o_delta_"+rel)); err != nil {
				return nil, err
			}
			res, err := tx.Exec(fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s EXCEPT SELECT %s FROM %s",
				quoteIdent("_o_delta_"+rel), c, c, quoteIdent("_o_new_"+rel), c, quoteIdent("_o_rule_"+rel)))
			if err != nil {
				return nil, err
			}
			n, _ := res.RowsAffected()
			total += n
			if _, err := tx.Exec(fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", quoteIdent("_o_rule_"+rel), c, c, quoteIdent("_o_delta_"+rel))); err != nil {
				return nil, err
			}
		}
		if total == 0 {
			break
		}
	}

	query, params := oracleWhere("SELECT "+colDefList(cols[relation])+" FROM "+quoteIdent("_o_rule_"+relation), "", constraints, nil)
	rows, err := tx.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTuples(rows, relation)
}

func headColumnsOf(rules []Rule) map[string][]string {
	cols := map[string][]string{}
	for _, r := range rules {
		if _, ok := cols[r.Head.Rel]; !ok {
			cols[r.Head.Rel] = sortedArgKeys(r.Head.Args)
		}
	}
	return cols
}

func oracleWhere(query, qualifier string, constraints map[string]interface{}, params []interface{}) (string, []interface{}) {
	var parts []string
	for _, k := range sortedConstraintKeys(constraints) {
		parts = append(parts, qualifier+quoteIdent(k)+" = ?")
		params = append(params, constraints[k])
	}
	if len(parts) > 0 {
		query += " WHERE " + strings.Join(parts, " AND ")
	}
	return query, params
}

func sortedConstraintKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

func oracleEDB(db *database.CatalogDB, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	facts, err := db.QueryFacts(database.FactFilter{Relation: relation, ValidAt: scope.ValidAt, TxAt: scope.TxAt})
	if err != nil {
		return nil, err
	}
	var out []Tuple
	for _, f := range facts {
		if scope.ValidAt == nil && scope.TxAt == nil && (f.TxEnd != nil || f.ValidEnd != nil) {
			continue
		}
		v, err := database.DecodeQueryJSON(f.Args)
		if err != nil {
			return nil, err
		}
		args, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		t := Tuple{Relation: relation, Args: args}
		if matchConstraints(t, constraints) {
			out = append(out, t)
		}
	}
	return out, nil
}

package datalog

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

const maxFixpointIterations = 100

func EvalRecursive(db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
	constraints, err := database.QueryConstraints(constraints)
	if err != nil {
		return nil, err
	}
	recRules, baseRules := PartitionRules(rules)

	derivedRels := derivedRelations(rules)
	headCols := headColumns(rules, derivedRels)

	tx, err := db.DB().Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := createTempTables(tx, derivedRels, headCols); err != nil {
		return nil, fmt.Errorf("create temp tables: %w", err)
	}

	if err := seedBase(tx, baseRules, derivedRels, headCols, scope); err != nil {
		return nil, fmt.Errorf("seed base: %w", err)
	}

	if err := fixpointLoop(tx, recRules, derivedRels, headCols, scope); err != nil {
		return nil, fmt.Errorf("fixpoint: %w", err)
	}

	results, err := extractResults(tx, relation, headCols, constraints)
	if err != nil {
		return nil, fmt.Errorf("extract results: %w", err)
	}

	return results, nil
}

func derivedRelations(rules []Rule) map[string]bool {
	rels := make(map[string]bool)
	for _, r := range rules {
		rels[r.Head.Rel] = true
	}
	return rels
}

func headColumns(rules []Rule, derived map[string]bool) map[string][]string {
	cols := make(map[string][]string)
	for _, r := range rules {
		rel := r.Head.Rel
		if _, done := cols[rel]; done {
			continue
		}
		keys := sortedArgKeys(r.Head.Args)
		var varKeys []string
		for _, k := range keys {
			if r.Head.Args[k].IsVariable() {
				varKeys = append(varKeys, k)
			}
		}
		cols[rel] = varKeys
	}
	return cols
}

func createTempTables(tx *sql.Tx, derived map[string]bool, headCols map[string][]string) error {
	for rel := range derived {
		cols := headCols[rel]
		if len(cols) == 0 {
			continue
		}
		colDef := colDefList(cols)
		pkDef := colDefList(cols)

		for _, tpl := range []string{"_rule_%s", "_delta_%s", "_new_%s"} {
			name := fmt.Sprintf(tpl, rel)
			ddl := fmt.Sprintf("CREATE TEMP TABLE \"%s\" (%s, PRIMARY KEY(%s))", name, colDef, pkDef)
			if _, err := tx.Exec(ddl); err != nil {
				return fmt.Errorf("create %s: %w", name, err)
			}
		}
	}
	return nil
}

func seedBase(tx *sql.Tx, baseRules []Rule, derived map[string]bool, headCols map[string][]string, scope TemporalScope) error {
	for _, rule := range baseRules {
		if !derived[rule.Head.Rel] {
			continue
		}
		cq, err := CompileWithOptions(rule, scope, CompileOptions{TableOverrides: builtinEDBTables})
		if err != nil {
			return fmt.Errorf("compile base rule %s: %w", rule.Name, err)
		}

		cols := headCols[rule.Head.Rel]
		colList := colDefList(cols)

		for _, prefix := range []string{"_rule_", "_delta_"} {
			stmt := fmt.Sprintf("INSERT OR IGNORE INTO \"%s%s\" (%s) %s", prefix, rule.Head.Rel, colList, cq.SQL)
			if _, err := tx.Exec(stmt, cq.Params...); err != nil {
				return fmt.Errorf("seed %s%s: %w", prefix, rule.Head.Rel, err)
			}
		}
	}
	return nil
}

// recursiveQueries compiles one semi-naive variant per derived body atom:
// that occurrence reads delta, and every other occurrence reads all known rows.
// A relation-level override alone cannot represent nonlinear self-joins.
func recursiveQueries(rules []Rule, derived map[string]bool, scope TemporalScope) ([]*CompiledQuery, error) {
	overrides := make(map[string]string, len(derived))
	for rel := range derived {
		overrides[rel] = fmt.Sprintf("\"_rule_%s\"", rel)
	}
	overrides = withBuiltinEDB(overrides)
	var queries []*CompiledQuery
	for _, rule := range rules {
		for i, atom := range rule.Body {
			if !derived[atom.Rel] {
				continue
			}
			cq, err := CompileWithOptions(rule, scope, CompileOptions{
				TableOverrides:     overrides,
				AtomTableOverrides: map[int]string{i: fmt.Sprintf("\"_delta_%s\"", atom.Rel)},
			})
			if err != nil {
				return nil, fmt.Errorf("compile recursive rule %s: %w", rule.Name, err)
			}
			queries = append(queries, cq)
		}
	}
	return queries, nil
}

func fixpointLoop(tx *sql.Tx, recRules []Rule, derived map[string]bool, headCols map[string][]string, scope TemporalScope) error {
	queries, err := recursiveQueries(recRules, derived, scope)
	if err != nil {
		return err
	}
	for iter := 0; iter < maxFixpointIterations; iter++ {
		var totalNew int64
		for rel := range derived {
			if _, err := tx.Exec(fmt.Sprintf("DELETE FROM \"_new_%s\"", rel)); err != nil {
				return fmt.Errorf("clear _new_%s: %w", rel, err)
			}
		}
		for _, cq := range queries {
			rel := cq.Head.Rel
			insertSQL := fmt.Sprintf("INSERT OR IGNORE INTO \"_new_%s\" (%s) %s", rel, colDefList(headCols[rel]), cq.SQL)
			if _, err := tx.Exec(insertSQL, cq.Params...); err != nil {
				return fmt.Errorf("derive %s iter %d: %w", rel, iter, err)
			}
		}

		// Compute each delta BEFORE merging it, so the next round only sees
		// genuinely new tuples. All rule variants above see the same round.
		for rel := range derived {
			cols := colDefList(headCols[rel])
			if _, err := tx.Exec(fmt.Sprintf("DELETE FROM \"_delta_%s\"", rel)); err != nil {
				return fmt.Errorf("clear delta %s: %w", rel, err)
			}
			deltaSQL := fmt.Sprintf(
				"INSERT INTO \"_delta_%s\" (%s) SELECT %s FROM \"_new_%s\" EXCEPT SELECT %s FROM \"_rule_%s\"",
				rel, cols, cols, rel, cols, rel)
			res, err := tx.Exec(deltaSQL)
			if err != nil {
				return fmt.Errorf("compute delta %s iter %d: %w", rel, iter, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("count delta %s: %w", rel, err)
			}
			totalNew += n
			mergeSQL := fmt.Sprintf("INSERT INTO \"_rule_%s\" (%s) SELECT %s FROM \"_delta_%s\"", rel, cols, cols, rel)
			if _, err := tx.Exec(mergeSQL); err != nil {
				return fmt.Errorf("merge delta %s iter %d: %w", rel, iter, err)
			}
		}
		if totalNew == 0 {
			return nil
		}
	}
	return fmt.Errorf("fixpoint not reached after %d iterations", maxFixpointIterations)
}

func extractResults(tx *sql.Tx, relation string, headCols map[string][]string, constraints map[string]interface{}) ([]Tuple, error) {
	cols, ok := headCols[relation]
	if !ok || len(cols) == 0 {
		return nil, nil
	}

	colList := colDefList(cols)
	query := fmt.Sprintf("SELECT %s FROM \"_rule_%s\"", colList, relation)

	var whereParts []string
	var params []interface{}
	for k, v := range constraints {
		whereParts = append(whereParts, fmt.Sprintf("\"%s\" = ?", strings.ReplaceAll(k, `"`, `""`)))
		params = append(params, v)
	}
	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}

	rows, err := tx.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTuples(rows, relation, cols)
}

func colDefList(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = fmt.Sprintf("\"%s\"", c)
	}
	return strings.Join(quoted, ", ")
}

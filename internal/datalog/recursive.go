package datalog

import (
	"database/sql"
	"fmt"
	"strings"

	"pudl/internal/database"
)

const maxFixpointIterations = 100

func EvalRecursive(db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error) {
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

func fixpointLoop(tx *sql.Tx, recRules []Rule, derived map[string]bool, headCols map[string][]string, scope TemporalScope) error {
	for iter := 0; iter < maxFixpointIterations; iter++ {
		totalNew := 0

		for rel := range derived {
			if _, err := tx.Exec(fmt.Sprintf("DELETE FROM \"_new_%s\"", rel)); err != nil {
				return fmt.Errorf("clear _new_%s: %w", rel, err)
			}
		}

		for _, rule := range recRules {
			overrides := make(map[string]string)
			for _, atom := range rule.Body {
				if derived[atom.Rel] {
					overrides[atom.Rel] = fmt.Sprintf("\"_delta_%s\"", atom.Rel)
				}
			}

			cq, err := CompileWithOptions(rule, scope, CompileOptions{TableOverrides: withBuiltinEDB(overrides)})
			if err != nil {
				return fmt.Errorf("compile recursive rule %s: %w", rule.Name, err)
			}

			cols := headCols[rule.Head.Rel]
			colList := colDefList(cols)

			// Insert into _new_, skipping rows already in _rule_
			insertSQL := fmt.Sprintf("INSERT OR IGNORE INTO \"_new_%s\" (%s) %s", rule.Head.Rel, colList, cq.SQL)
			if _, err := tx.Exec(insertSQL, cq.Params...); err != nil {
				return fmt.Errorf("insert _new_%s iter %d: %w", rule.Name, iter, err)
			}
		}

		// Move genuinely new rows from _new_ into _rule_ and count
		for rel := range derived {
			cols := headCols[rel]
			colList := colDefList(cols)

			insertSQL := fmt.Sprintf(
				"INSERT OR IGNORE INTO \"_rule_%s\" (%s) SELECT %s FROM \"_new_%s\"",
				rel, colList, colList, rel,
			)
			res, err := tx.Exec(insertSQL)
			if err != nil {
				return fmt.Errorf("merge _new_ to _rule_%s iter %d: %w", rel, iter, err)
			}
			n, _ := res.RowsAffected()
			totalNew += int(n)
		}

		if totalNew == 0 {
			return nil
		}

		// Rebuild delta: only the genuinely new rows (those that were just added to _rule_)
		for rel := range derived {
			cols := headCols[rel]
			colList := colDefList(cols)

			if _, err := tx.Exec(fmt.Sprintf("DELETE FROM \"_delta_%s\"", rel)); err != nil {
				return fmt.Errorf("clear delta %s: %w", rel, err)
			}
			// _new_ may contain rows already in _rule_ before this iteration.
			// The genuinely new ones are in _new_ AND were just inserted (RowsAffected counted them).
			// Since _rule_ has PK dedup, the rows in _new_ that are also new in _rule_ are the delta.
			// We can get them by: _new_ EXCEPT rows that were in _rule_ before. But we don't have the "before" snapshot.
			// Alternative: since _new_ was built from _delta_ joins, and _rule_ only grew, just use _new_ as delta.
			// This is slightly less efficient (may re-derive known facts) but still terminates because
			// _rule_ grows monotonically and is finite, so eventually no new rows ⇒ totalNew == 0.
			rebuildSQL := fmt.Sprintf(
				"INSERT OR IGNORE INTO \"_delta_%s\" (%s) SELECT %s FROM \"_new_%s\"",
				rel, colList, colList, rel,
			)
			if _, err := tx.Exec(rebuildSQL); err != nil {
				return fmt.Errorf("rebuild delta %s: %w", rel, err)
			}
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
		whereParts = append(whereParts, fmt.Sprintf("\"%s\" = ?", k))
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

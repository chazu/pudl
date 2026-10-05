package datalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
)

// evalStratified answers a plan that contains at least one cycle. Components
// are materialized into temp tables in dependency order: an acyclic component
// with one INSERT, a cyclic component by semi-naive iteration. Only the
// relations the query reads are evaluated. The temp tables live inside a
// transaction that is always rolled back, so nothing persists.
func evalStratifiedContext(ctx context.Context, db *database.CatalogDB, plan *queryPlan, constraints map[string]interface{}, scope TemporalScope, maxIterations int) ([]Tuple, error) {
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	overrides := withBuiltinEDB(nil)
	for _, c := range plan.components {
		for _, rel := range c.rels {
			if err := createTempTableContext(ctx, tx, "_rule_", rel, plan.headCols[rel]); err != nil {
				return nil, err
			}
			overrides[rel] = tempTable("_rule_", rel)
		}

		if !c.cyclic {
			rel := c.rels[0]
			body, params, err := unionSQL(plan.byHead[rel], scope, overrides)
			if err != nil {
				return nil, err
			}
			insert := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) %s", tempTable("_rule_", rel), colDefList(plan.headCols[rel]), body)
			if _, err := tx.ExecContext(ctx, insert, params...); err != nil {
				return nil, fmt.Errorf("materialize %s: %w", rel, err)
			}
			continue
		}

		if err := fixpointContext(ctx, tx, plan, c, overrides, scope, maxIterations); err != nil {
			return nil, err
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s", colDefList(plan.headCols[plan.relation]), tempTable("_rule_", plan.relation))
	query, params := whereConstraints(query, "", constraints, nil)
	rows, err := tx.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", plan.relation, err)
	}
	defer rows.Close()
	return scanTuples(rows, plan.relation)
}

// fixpoint evaluates one cyclic component by semi-naive iteration. Rules whose
// bodies read no member of the component seed it; every other rule runs once
// per member occurrence in its body, with that occurrence reading the last
// round's delta and every other derived occurrence reading all known rows.
// Lower components are already materialized and read in full.
func fixpointContext(ctx context.Context, tx *sql.Tx, plan *queryPlan, c component, overrides map[string]string, scope TemporalScope, maxIterations int) error {
	member := make(map[string]bool, len(c.rels))
	for _, rel := range c.rels {
		member[rel] = true
		for _, prefix := range []string{"_delta_", "_new_"} {
			if err := createTempTableContext(ctx, tx, prefix, rel, plan.headCols[rel]); err != nil {
				return err
			}
		}
	}

	var variants []*CompiledQuery
	for _, rel := range c.rels {
		for _, rule := range plan.byHead[rel] {
			recursive := false
			for i, atom := range rule.Body {
				if !member[atom.Rel] {
					continue
				}
				recursive = true
				cq, err := CompileWithOptions(rule, scope, CompileOptions{
					TableOverrides:     overrides,
					AtomTableOverrides: map[int]string{i: tempTable("_delta_", atom.Rel)},
				})
				if err != nil {
					return fmt.Errorf("compile recursive rule %s: %w", rule.Name, err)
				}
				variants = append(variants, cq)
			}
			if recursive {
				continue
			}
			cq, err := CompileWithOptions(rule, scope, CompileOptions{TableOverrides: overrides})
			if err != nil {
				return fmt.Errorf("compile base rule %s: %w", rule.Name, err)
			}
			for _, prefix := range []string{"_rule_", "_delta_"} {
				stmt := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) %s", tempTable(prefix, rel), colDefList(plan.headCols[rel]), cq.SQL)
				if _, err := tx.ExecContext(ctx, stmt, cq.Params...); err != nil {
					return fmt.Errorf("seed %s: %w", rel, err)
				}
			}
		}
	}

	for iter := 0; iter < maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, rel := range c.rels {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+tempTable("_new_", rel)); err != nil {
				return fmt.Errorf("clear new %s: %w", rel, err)
			}
		}
		for _, cq := range variants {
			rel := cq.Head.Rel
			insert := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) %s", tempTable("_new_", rel), colDefList(plan.headCols[rel]), cq.SQL)
			if _, err := tx.ExecContext(ctx, insert, cq.Params...); err != nil {
				return fmt.Errorf("derive %s iter %d: %w", rel, iter, err)
			}
		}

		// Compute every delta BEFORE merging it, so the next round only sees
		// genuinely new tuples. All rule variants above see the same round.
		var added int64
		for _, rel := range c.rels {
			cols := colDefList(plan.headCols[rel])
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+tempTable("_delta_", rel)); err != nil {
				return fmt.Errorf("clear delta %s: %w", rel, err)
			}
			res, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s EXCEPT SELECT %s FROM %s",
				tempTable("_delta_", rel), cols, cols, tempTable("_new_", rel), cols, tempTable("_rule_", rel)))
			if err != nil {
				return fmt.Errorf("compute delta %s iter %d: %w", rel, iter, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("count delta %s: %w", rel, err)
			}
			added += n
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s",
				tempTable("_rule_", rel), cols, cols, tempTable("_delta_", rel))); err != nil {
				return fmt.Errorf("merge delta %s iter %d: %w", rel, iter, err)
			}
		}
		if added == 0 {
			return nil
		}
	}
	return fmt.Errorf("fixpoint for %v not reached after %d iterations (raise the iteration limit for deeper recursion)", c.rels, maxIterations)
}

// createTempTable creates one role's temp table for a relation, keyed on all
// of its head columns so INSERT OR IGNORE deduplicates.
func createTempTableContext(ctx context.Context, tx *sql.Tx, prefix, rel string, cols []string) error {
	colDef := colDefList(cols)
	ddl := fmt.Sprintf("CREATE TEMP TABLE %s (%s, PRIMARY KEY(%s))", tempTable(prefix, rel), colDef, colDef)
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create %s%s: %w", prefix, rel, err)
	}
	return nil
}

// tempTable names a relation's temp table for one evaluation role ("_rule_",
// "_delta_" or "_new_"), quoted.
func tempTable(prefix, rel string) string {
	return quoteIdent(prefix + rel)
}

func colDefList(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	return strings.Join(quoted, ", ")
}

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
)

var (
	queryRuleFile      string
	queryAllWorkspace  bool
	queryAsOfValid     string
	queryAsOfTx        string
	queryList          bool
	queryTopo          bool
	queryMaxIterations int
	queryTimeout       time.Duration
)

var queryCmd = &cobra.Command{
	Use:          "query <relation> [--field=value ...]",
	Short:        "Query derived facts using Datalog rules",
	SilenceUsage: true,
	Long: `Evaluate Datalog rules over the fact store and catalog, then query results.

Rules are loaded from CUE files in:
  1. .pudl/schema/pudl/rules/    (repo-scoped, highest priority)
  2. ~/.pudl/schema/pudl/rules/  (global)

Repo-scoped rules shadow global rules with the same name.

Ad-hoc rules can be loaded from a file with -f.

Positional constraints filter results (field=value pairs). JSON numeric values
are parsed without rounding; other unquoted values are strings. To match a
numeric-looking string, retain JSON quotes, e.g. 'id="123"'.

Temporal modes (determined by which flags are set):
  (none)           Evaluate over current facts
  --as-of-valid    Evaluate over facts true at a point in time
  --as-of-tx       Evaluate over facts known at a point in time
  (both)           Evaluate over what was believed at --as-of-tx about --as-of-valid

Use --list to see the relations (rule heads + EDB facts) you can query and the
arg keys each expects. Use --topo to read a relation's from/to edges as a
topological run order (dependencies first); it errors on a cycle.

Examples:
    pudl query --list
    pudl query depends_transitive
    pudl query depends_transitive from=api
    pudl query impacted_by changed=network
    pudl query --topo model_depends_on
    pudl query -f my-analysis.cue corroborated_obstacle
    pudl query observation --as-of-valid 2026-04-01T14:30:00Z
    pudl query depends_transitive --json`,
	Args: func(cmd *cobra.Command, args []string) error {
		if queryList {
			if queryTopo {
				return fmt.Errorf("--list cannot be combined with --topo")
			}
			return cobra.NoArgs(cmd, args)
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if queryTimeout < 0 {
			return fmt.Errorf("--timeout must not be negative")
		}
		if queryMaxIterations < 0 {
			return fmt.Errorf("--max-iterations must not be negative")
		}
		ctx := cmd.Context()
		if queryTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, queryTimeout)
			defer cancel()
		}
		if queryList {
			return runQueryList()
		}
		relation := args[0]

		// Parse field constraints from remaining args (key=value pairs)
		constraints, err := parseQueryConstraints(args[1:])
		if err != nil {
			return err
		}

		// Open database
		configDir := effectivePudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		// Parse temporal flags
		var validAt, txAt *int64
		if queryAsOfValid != "" {
			t, err := parseTime(queryAsOfValid)
			if err != nil {
				return fmt.Errorf("invalid --as-of-valid: %w", err)
			}
			validAt = &t
		}
		if queryAsOfTx != "" {
			t, err := parseTime(queryAsOfTx)
			if err != nil {
				return fmt.Errorf("invalid --as-of-tx: %w", err)
			}
			txAt = &t
		}

		rules, err := loadQueryRules(configDir)
		if err != nil {
			return err
		}

		// Evaluate only the relation's dependency closure: one SQL statement
		// when it has no cycles, stratified fixpoint iteration when it does.
		scope := datalog.TemporalScope{ValidAt: validAt, TxAt: txAt}
		results, err := datalog.EvaluateContext(ctx, db, rules, relation, constraints, scope,
			datalog.EvalOptions{MaxIterations: queryMaxIterations})
		if err != nil {
			return err
		}

		if queryTopo {
			return printTopoOrder(relation, results)
		}

		if jsonOutput {
			// Convert tuples to JSON-friendly format
			out := []map[string]interface{}{}
			for _, t := range results {
				entry := map[string]interface{}{
					"relation": t.Relation,
					"args":     t.Args,
				}
				out = append(out, entry)
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Fprintln(outw(), string(data))
			return nil
		}

		if len(results) == 0 {
			fmt.Fprintln(outw(), "No results.")
			return nil
		}

		for _, t := range results {
			printTuple(t)
		}
		fmt.Fprintf(outw(), "\n%d result(s)\n", len(results))
		return nil
	},
}

func parseQueryConstraints(args []string) (map[string]interface{}, error) {
	constraints := make(map[string]interface{}, len(args))
	for _, arg := range args {
		key, raw, ok := strings.Cut(arg, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid constraint %q: expected field=value with a nonblank field", arg)
		}
		if _, exists := constraints[key]; exists {
			return nil, fmt.Errorf("duplicate constraint %q", key)
		}
		value, err := parseQueryConstraint(raw)
		if err != nil {
			return nil, fmt.Errorf("constraint %s: %w", key, err)
		}
		constraints[key] = value
	}
	return constraints, nil
}

func parseQueryConstraint(raw string) (interface{}, error) {
	// Preserve ordinary unquoted string operands, including identifiers such as
	// 00123 that are not JSON numbers. JSON quotes explicitly request a string.
	if !json.Valid([]byte(raw)) {
		return raw, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	switch v := value.(type) {
	case json.Number:
		return database.QueryNumber(v)
	case string:
		return v, nil
	default:
		return raw, nil
	}
}

func printTuple(t datalog.Tuple) {
	args, _ := json.Marshal(t.Args)
	fmt.Fprintf(outw(), "%s(%s)\n", t.Relation, string(args))
}

func loadRulesFromFile(path string) ([]datalog.Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return datalog.ParseRulesFromSource(string(data))
}

func init() {
	rootCmd.AddCommand(queryCmd)

	queryCmd.Flags().StringVarP(&queryRuleFile, "rule-file", "f", "", "Load additional rules from a CUE file")
	queryCmd.Flags().BoolVar(&queryAllWorkspace, "all-workspaces", false, "Include global rules and all workspace data")
	queryCmd.Flags().StringVar(&queryAsOfValid, "as-of-valid", "", "Evaluate over facts true at this time (RFC3339 or Unix)")
	queryCmd.Flags().StringVar(&queryAsOfTx, "as-of-tx", "", "Evaluate over facts known at this time (RFC3339 or Unix)")
	queryCmd.Flags().BoolVar(&queryList, "list", false, "List queryable relations (rule heads + EDB facts) and their arg keys")
	queryCmd.Flags().BoolVar(&queryTopo, "topo", false, "Read the relation's from/to edges as a topological run order (errors on a cycle)")
	queryCmd.Flags().DurationVar(&queryTimeout, "timeout", 0, "Maximum query duration (0 disables deadline)")
	queryCmd.Flags().IntVar(&queryMaxIterations, "max-iterations", datalog.DefaultMaxIterations, "Cap on fixpoint rounds per recursive cycle (the longest chain a recursive rule can follow)")

	queryCmd.ValidArgsFunction = completeRelations
}

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
)

var (
	queryRuleFile     string
	queryAllWorkspace bool
	queryAsOfValid    string
	queryAsOfTx       string
	queryList         bool
	queryTopo         bool
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

Positional constraints filter results (field=value pairs).

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
			return nil // --list takes no relation
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if queryList {
			return runQueryList()
		}
		relation := args[0]

		// Parse field constraints from remaining args (key=value pairs)
		constraints := make(map[string]interface{})
		for _, arg := range args[1:] {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				constraints[parts[0]] = parts[1]
			}
		}

		// Open database
		configDir := config.GetPudlDir()
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

		// Evaluate: SQL for non-recursive rules, recursive fixpoint fallback.
		scope := datalog.TemporalScope{ValidAt: validAt, TxAt: txAt}
		results, err := datalog.Evaluate(db, rules, relation, constraints, scope)
		if err != nil {
			return err
		}

		if queryTopo {
			return printTopoOrder(relation, results)
		}

		if jsonOutput {
			// Convert tuples to JSON-friendly format
			var out []map[string]interface{}
			for _, t := range results {
				entry := map[string]interface{}{
					"relation": t.Relation,
					"args":     t.Args,
				}
				out = append(out, entry)
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		if len(results) == 0 {
			fmt.Println("No results.")
			return nil
		}

		for _, t := range results {
			printTuple(t)
		}
		fmt.Printf("\n%d result(s)\n", len(results))
		return nil
	},
}

func printTuple(t datalog.Tuple) {
	args, _ := json.Marshal(t.Args)
	fmt.Printf("%s(%s)\n", t.Relation, string(args))
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

	queryCmd.ValidArgsFunction = completeRelations
}

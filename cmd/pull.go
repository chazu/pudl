package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
)

var (
	pullKind     string
	pullSource   string
	pullRelation string
)

var pullCmd = &cobra.Command{
	Use:   "pull [scope]",
	Short: "Retrieve all facts related to a scope or entity",
	Long: `Retrieve facts from the store filtered by scope, kind, source, or relation.

The positional argument matches against the scope field using prefix matching,
so "procyon-park" matches all scopes starting with "procyon-park" (including
"procyon-park:src/cli", "procyon-park:src/api", etc.).

Examples:
    pudl pull procyon-park:src/cli
    pudl pull procyon-park
    pudl pull --kind bug
    pudl pull --source claude-code
    pudl pull procyon-park --kind bug
    pudl pull maggie:vm --json
    pudl pull --relation observation --kind obstacle`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var scope string
		if len(args) > 0 {
			scope = args[0]
		}

		if scope == "" && pullKind == "" && pullSource == "" && pullRelation == "" {
			return fmt.Errorf("at least one filter required: scope (positional), --kind, --source, or --relation")
		}

		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		facts, err := queryPull(db, scope, pullKind, pullSource, pullRelation)
		if err != nil {
			return fmt.Errorf("query failed: %w", err)
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(facts, "", "  ")
			fmt.Println(string(out))
			return nil
		}

		if len(facts) == 0 {
			fmt.Println("No facts found.")
			return nil
		}

		printPullResults(facts)
		return nil
	},
}

func queryPull(db *database.CatalogDB, scope, kind, source, relation string) ([]database.Fact, error) {
	var conditions []string
	var params []interface{}

	// Only current facts (not retracted, still valid)
	conditions = append(conditions, "valid_end IS NULL", "tx_end IS NULL")

	if scope != "" {
		conditions = append(conditions, "json_extract(args, '$.scope') LIKE ?")
		params = append(params, scope+"%")
	}

	if kind != "" {
		conditions = append(conditions, "json_extract(args, '$.kind') = ?")
		params = append(params, kind)
	}

	if source != "" {
		conditions = append(conditions, "source = ?")
		params = append(params, source)
	}

	if relation != "" {
		conditions = append(conditions, "relation = ?")
		params = append(params, relation)
	}

	query := fmt.Sprintf(
		"SELECT id, relation, args, valid_start, valid_end, tx_start, tx_end, source, provenance FROM facts WHERE %s ORDER BY valid_start DESC",
		strings.Join(conditions, " AND "),
	)

	rows, err := db.DB().Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []database.Fact
	for rows.Next() {
		var f database.Fact
		if err := rows.Scan(&f.ID, &f.Relation, &f.Args, &f.ValidStart, &f.ValidEnd, &f.TxStart, &f.TxEnd, &f.Source, &f.Provenance); err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

func printPullResults(facts []database.Fact) {
	// Group by scope
	type group struct {
		scope string
		facts []database.Fact
	}
	groups := make(map[string]*group)
	var order []string

	for _, f := range facts {
		var obj map[string]interface{}
		json.Unmarshal([]byte(f.Args), &obj)
		scope, _ := obj["scope"].(string)
		if scope == "" {
			scope = "(global)"
		}
		if _, ok := groups[scope]; !ok {
			groups[scope] = &group{scope: scope}
			order = append(order, scope)
		}
		groups[scope].facts = append(groups[scope].facts, f)
	}

	for i, scopeKey := range order {
		g := groups[scopeKey]
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("── %s (%d) ──\n", g.scope, len(g.facts))
		for _, f := range g.facts {
			var obj map[string]interface{}
			json.Unmarshal([]byte(f.Args), &obj)
			kind, _ := obj["kind"].(string)
			desc, _ := obj["description"].(string)
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			ts := time.Unix(f.ValidStart, 0).Format("2006-01-02")
			fmt.Printf("  [%-12s] %s  (%s, %s)\n", kind, desc, f.Source, ts)
		}
	}

	fmt.Printf("\n%d fact(s)\n", len(facts))
}

func init() {
	rootCmd.AddCommand(pullCmd)

	pullCmd.Flags().StringVar(&pullKind, "kind", "", "Filter by observation kind (bug, obstacle, pattern, etc.)")
	pullCmd.Flags().StringVar(&pullSource, "source", "", "Filter by source")
	pullCmd.Flags().StringVar(&pullRelation, "relation", "", "Filter by relation (default: all)")
}

package cmd

import (
	"encoding/json"
	"fmt"
	"os/user"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/validator"
)

var (
	factsAddRelation string
	factsAddArgs     string
	factsAddSource   string
	factsAddSchema   string

	factsSearchRelation string
	factsSearchLimit    int
)

// defaultFactSource returns the current OS username, or "human" if unavailable.
// Shared default for fact writes.
func defaultFactSource() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "human"
}

var factsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a fact to the bitemporal store (the canonical write)",
	Long: `Add a fact under any relation using a JSON object. Facts are general-purpose.
Use --schema to validate the args against an authored CUE schema before storing them.

Examples:
    pudl facts add --relation depends --args '{"from":"api","to":"database"}'
    pudl facts add --relation config --args '{"key":"timeout","value":30}' --source operator`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if factsAddRelation == "" {
			return fmt.Errorf("--relation is required")
		}
		if database.IsReservedRelation(factsAddRelation) {
			return fmt.Errorf("relation %q is reserved and cannot be written directly", factsAddRelation)
		}

		// Args must parse to a JSON object.
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(factsAddArgs), &obj); err != nil {
			return fmt.Errorf("--args must be a JSON object: %w", err)
		}

		if obj == nil {
			return fmt.Errorf("--args must be a JSON object")
		}

		// Validation is explicit; relations carry no built-in memory policy.
		if factsAddSchema != "" {
			cfg, err := loadEffectiveConfig()
			if err != nil {
				return fmt.Errorf("failed to load config for validation: %w", err)
			}
			vs, err := validator.NewValidationService(effectiveSchemaPaths(cfg)...)
			if err != nil {
				return fmt.Errorf("failed to initialize validation: %w", err)
			}
			result := vs.ValidateDataAgainstSchema(obj, factsAddSchema)
			if !result.Valid {
				return fmt.Errorf("args do not satisfy %s:\n%s", factsAddSchema, vs.GetValidationSummary(result))
			}
		}

		configDir := effectivePudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		f, err := db.AddFact(database.Fact{
			Relation: factsAddRelation,
			Args:     factsAddArgs,
			Source:   factsAddSource,
		})
		if err != nil {
			return fmt.Errorf("failed to add fact: %w", err)
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(f, "", "  ")
			fmt.Fprintln(outw(), string(out))
		} else {
			fmt.Fprintf(outw(), "Added fact %s\n", f.ID[:12])
		}
		return nil
	},
}

var factsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search over currently-valid facts",
	Long: `Keyword search over the indexed text of currently-valid facts, best matches
first. The index covers the values of each fact's args (not the JSON keys).

The query uses SQLite FTS5 syntax: bare terms are ANDed, "quoted phrases" match
in order, a trailing * is a prefix match, and AND/OR/NOT combine terms.

Examples:
    pudl facts search "rate limiting"
    pudl facts search "circular dependency" --relation observation
    pudl facts search "auth*" --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		configDir := effectivePudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		facts, err := db.SearchCurrentFacts(args[0], factsSearchRelation, factsSearchLimit)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(facts, "", "  ")
			fmt.Fprintln(outw(), string(out))
			return nil
		}
		if len(facts) == 0 {
			fmt.Fprintln(outw(), "No matching facts.")
			return nil
		}
		for _, f := range facts {
			printFact(f, false)
		}
		fmt.Fprintf(outw(), "\n%d match(es)\n", len(facts))
		return nil
	},
}

func init() {
	factsCmd.AddCommand(factsAddCmd)
	factsCmd.AddCommand(factsSearchCmd)

	factsSearchCmd.Flags().StringVar(&factsSearchRelation, "relation", "", "Limit search to a relation")
	factsSearchCmd.Flags().IntVar(&factsSearchLimit, "limit", 20, "Maximum results (0 = no limit)")
	factsSearchCmd.RegisterFlagCompletionFunc("relation", completeRelations)

	factsAddCmd.Flags().StringVar(&factsAddRelation, "relation", "", "Relation to write (required)")
	factsAddCmd.Flags().StringVar(&factsAddArgs, "args", "", "Fact body as a JSON object (required)")
	factsAddCmd.Flags().StringVar(&factsAddSource, "source", defaultFactSource(), "Source of the fact (agent name or username)")
	factsAddCmd.Flags().StringVar(&factsAddSchema, "schema", "", "Validate args against a named on-disk CUE schema (e.g. user/config.#Setting)")
	factsAddCmd.MarkFlagRequired("relation")
	factsAddCmd.MarkFlagRequired("args")
	factsAddCmd.RegisterFlagCompletionFunc("relation", completeRelations)
}

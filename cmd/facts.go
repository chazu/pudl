package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
)

var factsCmd = &cobra.Command{
	Use:   "facts",
	Short: "Query the bitemporal fact store",
	Long: `Query facts stored in the bitemporal fact store.

Facts are typed assertions — observations, dependencies, derived facts — with
full valid-time and transaction-time tracking.

Available subcommands:
- list:       Query facts by relation with temporal filtering
- show:       Inspect a single fact by ID
- retract:    Mark a fact as retracted (we were wrong)
- invalidate: Mark a fact as no longer valid (reality changed)

Examples:
    pudl facts list --relation observation
    pudl facts show abc123...
    pudl facts retract abc123...
    pudl facts invalidate abc123...`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var (
	factsRelation   string
	factsSource     string
	factsAsOfValid  string
	factsAsOfTx     string
	factsVerbose    bool
)

var factsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List facts from the bitemporal store",
	Long: `Query facts by relation with optional temporal and source filtering.

Temporal modes (determined by which flags are set):
  (none)           Current facts: valid now, not retracted
  --as-of-valid    What was true at a point in time (current knowledge)
  --as-of-tx       What we believed at a point in time
  (both)           What we believed at --as-of-tx about what was true at --as-of-valid

Time format: RFC3339 (e.g. 2026-04-01T14:30:00Z) or Unix timestamp.

Examples:
    pudl facts list --relation observation
    pudl facts list --relation observation --source claude-code
    pudl facts list --relation depends --as-of-valid 2026-04-01T14:30:00Z
    pudl facts list --relation observation --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if factsRelation == "" {
			return fmt.Errorf("--relation is required")
		}

		filter := database.FactFilter{
			Relation: factsRelation,
		}

		// Parse temporal flags
		if factsAsOfValid != "" {
			t, err := parseTime(factsAsOfValid)
			if err != nil {
				return fmt.Errorf("invalid --as-of-valid: %w", err)
			}
			filter.ValidAt = &t
		}
		if factsAsOfTx != "" {
			t, err := parseTime(factsAsOfTx)
			if err != nil {
				return fmt.Errorf("invalid --as-of-tx: %w", err)
			}
			filter.TxAt = &t
		}

		// Open database
		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		facts, err := db.QueryFacts(filter)
		if err != nil {
			return fmt.Errorf("failed to query facts: %w", err)
		}

		// Filter by source if specified (post-query filter)
		if factsSource != "" {
			var filtered []database.Fact
			for _, f := range facts {
				if f.Source == factsSource {
					filtered = append(filtered, f)
				}
			}
			facts = filtered
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

		for _, f := range facts {
			printFact(f, factsVerbose)
		}

		fmt.Printf("\n%d fact(s)\n", len(facts))
		return nil
	},
}

func printFact(f database.Fact, verbose bool) {
	validStart := time.Unix(f.ValidStart, 0).Format(time.RFC3339)

	if verbose {
		fmt.Printf("ID:       %s\n", f.ID)
		fmt.Printf("Relation: %s\n", f.Relation)
		fmt.Printf("Args:     %s\n", f.Args)
		fmt.Printf("Source:   %s\n", f.Source)
		fmt.Printf("Valid:    %s", validStart)
		if f.ValidEnd != nil {
			fmt.Printf(" → %s", time.Unix(*f.ValidEnd, 0).Format(time.RFC3339))
		}
		fmt.Println()
		fmt.Printf("Tx:       %s", time.Unix(f.TxStart, 0).Format(time.RFC3339))
		if f.TxEnd != nil {
			fmt.Printf(" → %s (retracted)", time.Unix(*f.TxEnd, 0).Format(time.RFC3339))
		}
		fmt.Println()
		if f.Provenance != "" {
			fmt.Printf("Prov:     %s\n", f.Provenance)
		}
		fmt.Println("---")
	} else {
		// Compact format: ID (short) | relation | source | timestamp | key details from args
		idShort := f.ID[:12]
		summary := extractArgsSummary(f.Args)
		fmt.Printf("%-12s  %-14s  %-12s  %s  %s\n", idShort, f.Relation, f.Source, validStart, summary)
	}
}

// extractArgsSummary pulls a human-readable summary from JSON args.
func extractArgsSummary(argsJSON string) string {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &obj); err != nil {
		return argsJSON
	}

	// For observations, show kind + truncated description
	if kind, ok := obj["kind"].(string); ok {
		if desc, ok := obj["description"].(string); ok {
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			return fmt.Sprintf("[%s] %s", kind, desc)
		}
		return fmt.Sprintf("[%s]", kind)
	}

	// For other relations, show first two key=value pairs
	var parts []string
	for k, v := range obj {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		if len(parts) >= 2 {
			break
		}
	}
	return fmt.Sprintf("{%s}", joinStrings(parts, ", "))
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

// parseTime parses RFC3339 or Unix timestamp strings.
func parseTime(s string) (int64, error) {
	// Try RFC3339 first
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t.Unix(), nil
	}

	// Try Unix timestamp
	var unix int64
	_, err = fmt.Sscanf(s, "%d", &unix)
	if err == nil {
		return unix, nil
	}

	return 0, fmt.Errorf("expected RFC3339 (e.g. 2026-04-01T14:30:00Z) or Unix timestamp, got %q", s)
}

var factsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show details of a single fact",
	Long: `Display full details of a fact by its ID.

Accepts the full 64-character hex ID or a unique prefix.

Examples:
    pudl facts show c0b4392d347aca8e517c9bb8775a78951c6214e07f04f9168dbe3fbfa862e32a
    pudl facts show c0b4392d347a
    pudl facts show c0b4392d347a --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		// Try exact match first, then prefix match
		f, err := db.GetFact(id)
		if err != nil {
			f, err = db.GetFactByPrefix(id)
			if err != nil {
				return fmt.Errorf("fact not found: %s", id)
			}
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(f, "", "  ")
			fmt.Println(string(out))
			return nil
		}

		printFact(*f, true)
		return nil
	},
}

var factsRetractCmd = &cobra.Command{
	Use:   "retract <id>",
	Short: "Retract a fact (mark as no longer asserted)",
	Long: `Mark a fact as retracted by setting its transaction end time.

Use this when a fact was recorded in error — the assertion was wrong.
The fact remains in the store for audit purposes but disappears from
current queries.

Examples:
    pudl facts retract c0b4392d347aca8e...`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		// Resolve prefix if needed
		resolved, err := resolveFactID(db, id)
		if err != nil {
			return err
		}

		if err := db.RetractFact(resolved); err != nil {
			return fmt.Errorf("failed to retract: %w", err)
		}

		fmt.Printf("Retracted fact %s\n", resolved[:12])
		return nil
	},
}

var factsInvalidateCmd = &cobra.Command{
	Use:   "invalidate <id>",
	Short: "Invalidate a fact (mark as no longer true)",
	Long: `Mark a fact as no longer valid by setting its valid end time.

Use this when reality changed — the fact was correct when recorded but
is no longer true. The fact remains visible in historical queries
(--as-of-valid) but disappears from current queries.

Examples:
    pudl facts invalidate c0b4392d347aca8e...`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		resolved, err := resolveFactID(db, id)
		if err != nil {
			return err
		}

		if err := db.InvalidateFact(resolved); err != nil {
			return fmt.Errorf("failed to invalidate: %w", err)
		}

		fmt.Printf("Invalidated fact %s\n", resolved[:12])
		return nil
	},
}

// resolveFactID tries exact match then prefix match for a fact ID.
func resolveFactID(db *database.CatalogDB, id string) (string, error) {
	f, err := db.GetFact(id)
	if err == nil {
		return f.ID, nil
	}
	f, err = db.GetFactByPrefix(id)
	if err != nil {
		return "", fmt.Errorf("fact not found: %s", id)
	}
	return f.ID, nil
}

func init() {
	rootCmd.AddCommand(factsCmd)
	factsCmd.AddCommand(factsListCmd)
	factsCmd.AddCommand(factsShowCmd)
	factsCmd.AddCommand(factsRetractCmd)
	factsCmd.AddCommand(factsInvalidateCmd)

	factsListCmd.Flags().StringVar(&factsRelation, "relation", "", "Relation to query (required)")
	factsListCmd.Flags().StringVar(&factsSource, "source", "", "Filter by source")
	factsListCmd.Flags().StringVar(&factsAsOfValid, "as-of-valid", "", "Query valid time (RFC3339 or Unix timestamp)")
	factsListCmd.Flags().StringVar(&factsAsOfTx, "as-of-tx", "", "Query transaction time (RFC3339 or Unix timestamp)")
	factsListCmd.Flags().BoolVarP(&factsVerbose, "verbose", "v", false, "Show full fact details")
}

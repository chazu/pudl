package cmd

import (
	"encoding/json"
	"fmt"
	"os/user"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
)

var (
	observeKind   string
	observeScope  []string
	observeSource string
)

var observeCmd = &cobra.Command{
	Use:   "observe <description>",
	Short: "Record a structured observation about the codebase",
	Long: `Record an observation as a fact in the bitemporal fact store.

Observations are structured assertions about the codebase — patterns noticed,
obstacles encountered, suggestions for improvement. They serve as raw material
for the nous reasoning engine and Datalog-derived analysis.

Kinds:
  fact          A verified truth about the system
  obstacle      Something blocking progress
  pattern       A recurring structure or behavior
  antipattern   A recurring problem
  suggestion    A proposed improvement
  bug           A defect
  opportunity   A potential enhancement

Examples:
    pudl observe "auth package has circular dependency with user package" --kind obstacle --scope pkg/auth,pkg/user
    pudl observe "all database calls go through a single connection pool" --kind pattern
    pudl observe "error handling in API layer is inconsistent" --kind antipattern --scope cmd/api
    pudl observe "the Config struct has 47 fields, should be split" --kind suggestion --scope internal/config`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		description := args[0]
		if strings.TrimSpace(description) == "" {
			return fmt.Errorf("description cannot be empty")
		}

		// Build observation args
		obs := map[string]interface{}{
			"kind":        observeKind,
			"description": description,
			"source":      observeSource,
			"status":      "raw",
			"worth":       0.5,
		}
		if len(observeScope) > 0 {
			obs["scope"] = observeScope
		}

		argsJSON, err := json.Marshal(obs)
		if err != nil {
			return fmt.Errorf("failed to marshal observation: %w", err)
		}

		// Open database
		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		// Store as fact
		f, err := db.AddFact(database.Fact{
			Relation: "observation",
			Args:     string(argsJSON),
			Source:   observeSource,
		})
		if err != nil {
			// Content-addressed dedup: same observation from same source = PK conflict
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				fmt.Println("Observation already recorded (duplicate)")
				return nil
			}
			return fmt.Errorf("failed to record observation: %w", err)
		}

		fmt.Printf("Recorded observation %s\n", f.ID[:12])
		if jsonOutput {
			out, _ := json.MarshalIndent(f, "", "  ")
			fmt.Println(string(out))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(observeCmd)

	observeCmd.Flags().StringVar(&observeKind, "kind", "fact", "Observation kind (fact, obstacle, pattern, antipattern, suggestion, bug, opportunity)")
	observeCmd.Flags().StringSliceVar(&observeScope, "scope", nil, "Scope: file paths, package names, or module names")

	// Default source to current OS user
	defaultSource := "human"
	if u, err := user.Current(); err == nil {
		defaultSource = u.Username
	}
	observeCmd.Flags().StringVar(&observeSource, "source", defaultSource, "Source of the observation (agent name or username)")
}

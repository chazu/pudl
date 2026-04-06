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
	observeScope  string
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

Scope format:
  Use "repo:path" to identify where the observation applies.
  The repo name is the repository, and the path is the package
  or file within it. Omit the path for repo-wide observations.

  Examples:
    pudl:internal/database      — a specific package in the pudl repo
    pudl:cmd/api                — the API command package
    nous:internal/engine        — the nous engine package
    pudl                        — the pudl repo as a whole
    myapp:pkg/auth              — a package in another repo

  Agents: always use this format so observations are globally
  unambiguous and joinable by Datalog rules across repositories.

Examples:
    pudl observe "auth has circular dep with user" --kind obstacle --scope pudl:pkg/auth
    pudl observe "all database calls use single pool" --kind pattern --scope pudl:internal/db
    pudl observe "error handling is inconsistent" --kind antipattern --scope pudl:cmd/api
    pudl observe "Config struct has 47 fields" --kind suggestion --scope pudl:internal/config
    pudl observe "all repos should use golangci-lint" --kind suggestion`,
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
		if observeScope != "" {
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
	observeCmd.Flags().StringVar(&observeScope, "scope", "", "Scope as repo:path (e.g. pudl:internal/database, nous:internal/engine)")

	// Default source to current OS user
	defaultSource := "human"
	if u, err := user.Current(); err == nil {
		defaultSource = u.Username
	}
	observeCmd.Flags().StringVar(&observeSource, "source", defaultSource, "Source of the observation (agent name or username)")
}

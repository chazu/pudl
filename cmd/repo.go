package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/definition"
	"pudl/internal/errors"
	"pudl/internal/model"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Repository-wide operations",
	Long: `Operations that span the entire schema repository.

Available subcommands:
- validate: Validate all schemas, models, and definitions

Examples:
    pudl repo validate`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var repoValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate all schemas, models, and definitions",
	Long: `Run workspace-wide validation across all schemas, models, and definitions.

Reports total counts, validation errors, and broken socket wiring.

Examples:
    pudl repo validate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoValidateCommand()
	},
}

func init() {
	rootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoValidateCmd)
}

func runRepoValidateCommand() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	fmt.Println("Repository Validation")
	fmt.Println("=====================")
	fmt.Println()

	hasErrors := false

	// Models
	modelDiscoverer := model.NewDiscoverer(cfg.SchemaPath)
	models, err := modelDiscoverer.ListModels()
	if err != nil {
		fmt.Printf("  Models:      error loading (%v)\n", err)
		hasErrors = true
	} else {
		fmt.Printf("  Models:      %d found\n", len(models))
	}

	// Definitions
	defDiscoverer := definition.NewDiscoverer(cfg.SchemaPath)
	definitions, err := defDiscoverer.ListDefinitions()
	if err != nil {
		fmt.Printf("  Definitions: error loading (%v)\n", err)
		hasErrors = true
	} else {
		fmt.Printf("  Definitions: %d found\n", len(definitions))

		// Check socket wiring
		if len(definitions) > 0 {
			graph := definition.BuildGraph(definitions)
			_, sortErr := graph.TopologicalSort()
			if sortErr != nil {
				fmt.Printf("  Wiring:      ERROR - %v\n", sortErr)
				hasErrors = true
			} else {
				// Count wired bindings
				totalBindings := 0
				for _, d := range definitions {
					totalBindings += len(d.SocketBindings)
				}
				fmt.Printf("  Wiring:      %d socket bindings, no cycles\n", totalBindings)
			}
		}
	}

	// Definition validation
	if len(definitions) > 0 {
		defValidator := definition.NewValidator(cfg.SchemaPath)
		results, err := defValidator.ValidateAll()
		if err != nil {
			fmt.Printf("  Validation:  error (%v)\n", err)
			hasErrors = true
		} else {
			passCount := 0
			failCount := 0
			for _, r := range results {
				if r.Valid {
					passCount++
				} else {
					failCount++
				}
			}
			if failCount > 0 {
				fmt.Printf("  Validation:  %d passed, %d failed\n", passCount, failCount)
				hasErrors = true
			} else {
				fmt.Printf("  Validation:  all %d definitions valid\n", passCount)
			}
		}
	}

	fmt.Println()
	if hasErrors {
		fmt.Println("Result: ISSUES FOUND")
		return errors.NewValidationError("repository", nil, fmt.Errorf("validation issues detected"))
	}

	fmt.Println("Result: ALL CLEAR")
	return nil
}

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/definition"
	"pudl/internal/errors"
)

var definitionGraphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Show definition dependency graph",
	Long: `Display the dependency graph between definitions based on socket wiring.

Shows topological ordering (execution order) and dependency relationships.

Examples:
    pudl definition graph`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDefinitionGraphCommand()
	},
}

func init() {
	definitionCmd.AddCommand(definitionGraphCmd)
}

func runDefinitionGraphCommand() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	discoverer := definition.NewDiscoverer(cfg.SchemaPath)
	definitions, err := discoverer.ListDefinitions()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to list definitions", err)
	}

	if len(definitions) == 0 {
		fmt.Println("No definitions found.")
		return nil
	}

	graph := definition.BuildGraph(definitions)

	sorted, err := graph.TopologicalSort()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("\nDefinitions with circular dependencies cannot be ordered.")
		return nil
	}

	fmt.Println("Definition Dependency Graph:")
	fmt.Println()
	fmt.Println("Execution Order (dependencies first):")
	fmt.Println()

	for i, name := range sorted {
		deps := graph.GetDependencies(name)
		depStr := ""
		if len(deps) > 0 {
			depStr = fmt.Sprintf(" <- depends on: %s", strings.Join(deps, ", "))
		}
		fmt.Printf("  %d. %s%s\n", i+1, name, depStr)
	}

	fmt.Printf("\nTotal: %d definitions\n", len(sorted))
	return nil
}

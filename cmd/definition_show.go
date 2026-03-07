package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/definition"
	"pudl/internal/errors"
)

var definitionShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Display definition details",
	Long: `Display detailed information about a definition including model reference,
file path, and socket bindings.

Examples:
    pudl definition show my_simple
    pudl def show prod_instance`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeDefinitionNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDefinitionShowCommand(args[0])
	},
}

func init() {
	definitionCmd.AddCommand(definitionShowCmd)
}

func runDefinitionShowCommand(defName string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	discoverer := definition.NewDiscoverer(cfg.SchemaPath)
	def, err := discoverer.GetDefinition(defName)
	if err != nil {
		return errors.NewInputError(
			fmt.Sprintf("Definition not found: %s", defName),
			"Check available definitions with 'pudl definition list'",
		)
	}

	// Header
	fmt.Printf("Definition: %s\n", def.Name)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()

	fmt.Printf("  Model:    %s\n", def.ModelRef)
	fmt.Printf("  Package:  %s\n", def.Package)
	fmt.Printf("  File:     %s\n", def.FilePath)
	fmt.Println()

	// Socket bindings
	if len(def.SocketBindings) > 0 {
		fmt.Println("Socket Bindings:")
		var fields []string
		for field := range def.SocketBindings {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			fmt.Printf("  %s -> %s\n", field, def.SocketBindings[field])
		}
		fmt.Println()
	}

	// Show dependency info from graph
	allDefs, err := discoverer.ListDefinitions()
	if err == nil && len(allDefs) > 0 {
		graph := definition.BuildGraph(allDefs)

		deps := graph.GetDependencies(def.Name)
		if len(deps) > 0 {
			fmt.Printf("Depends on: %s\n", strings.Join(deps, ", "))
		}

		dependents := graph.GetDependents(def.Name)
		if len(dependents) > 0 {
			fmt.Printf("Depended on by: %s\n", strings.Join(dependents, ", "))
		}

		if len(deps) > 0 || len(dependents) > 0 {
			fmt.Println()
		}
	}

	return nil
}

func completeDefinitionNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	discoverer := definition.NewDiscoverer(cfg.SchemaPath)
	definitions, err := discoverer.ListDefinitions()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, d := range definitions {
		if toComplete == "" || strings.HasPrefix(d.Name, toComplete) {
			completions = append(completions, d.Name+"\t"+d.ModelRef)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

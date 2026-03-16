package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/definition"
	"pudl/internal/errors"
)

var definitionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available definitions",
	Long: `List all available definitions showing name, schema reference, and socket bindings.

Examples:
    pudl definition list
    pudl definition list --verbose
    pudl definition list --schema examples.#EC2Instance`,
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)
		if err := runDefinitionListCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	definitionCmd.AddCommand(definitionListCmd)
	definitionListCmd.Flags().BoolVarP(&defVerbose, "verbose", "v", false, "Show detailed information")
	definitionListCmd.Flags().StringVar(&defSchema, "schema", "", "Filter by schema reference")
}

func runDefinitionListCommand() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	discoverer := definition.NewDiscoverer(cfg.SchemaPath)
	definitions, err := discoverer.ListDefinitions()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to list definitions", err)
	}

	// Filter by model if specified
	if defSchema != "" {
		var filtered []definition.DefinitionInfo
		for _, d := range definitions {
			if d.SchemaRef == defSchema {
				filtered = append(filtered, d)
			}
		}
		definitions = filtered
	}

	if len(definitions) == 0 {
		fmt.Println("No definitions found.")
		if defSchema != "" {
			fmt.Printf("\nNo definitions for schema '%s'. Try: pudl definition list\n", defSchema)
		}
		return nil
	}

	fmt.Println("Available Definitions:")
	fmt.Println()

	for _, d := range definitions {
		if defVerbose {
			fmt.Printf("  %s\n", d.Name)
			fmt.Printf("    Schema:   %s\n", d.SchemaRef)
			fmt.Printf("    Package:  %s\n", d.Package)
			fmt.Printf("    File:     %s\n", d.FilePath)
			if len(d.SocketBindings) > 0 {
				fmt.Printf("    Bindings:\n")
				for field, ref := range d.SocketBindings {
					fmt.Printf("      %s -> %s\n", field, ref)
				}
			}
			fmt.Println()
		} else {
			bindingStr := ""
			if len(d.SocketBindings) > 0 {
				bindingStr = fmt.Sprintf("  %d bindings", len(d.SocketBindings))
			}
			fmt.Printf("  %-30s %-40s%s\n", d.Name, d.SchemaRef, bindingStr)
		}
	}

	fmt.Printf("\nTotal: %d definitions\n", len(definitions))
	return nil
}

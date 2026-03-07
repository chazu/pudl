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
	Long: `List all available definitions showing name, model reference, and socket bindings.

Examples:
    pudl definition list
    pudl definition list --verbose
    pudl definition list --model examples.#EC2InstanceModel`,
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
	definitionListCmd.Flags().StringVar(&defModel, "model", "", "Filter by model reference")
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
	if defModel != "" {
		var filtered []definition.DefinitionInfo
		for _, d := range definitions {
			if d.ModelRef == defModel {
				filtered = append(filtered, d)
			}
		}
		definitions = filtered
	}

	if len(definitions) == 0 {
		fmt.Println("No definitions found.")
		if defModel != "" {
			fmt.Printf("\nNo definitions for model '%s'. Try: pudl definition list\n", defModel)
		}
		return nil
	}

	fmt.Println("Available Definitions:")
	fmt.Println()

	for _, d := range definitions {
		if defVerbose {
			fmt.Printf("  %s\n", d.Name)
			fmt.Printf("    Model:    %s\n", d.ModelRef)
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
			fmt.Printf("  %-30s %-40s%s\n", d.Name, d.ModelRef, bindingStr)
		}
	}

	fmt.Printf("\nTotal: %d definitions\n", len(definitions))
	return nil
}

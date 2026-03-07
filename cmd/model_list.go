package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/model"
)

// modelListCmd represents the model list command
var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available models",
	Long: `List all available models showing name, category, methods, and sockets.

Filtering Options:
- --category: Show only models in a specific category

Examples:
    pudl model list
    pudl model list --category compute
    pudl model list --verbose`,
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)
		if err := runModelListCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	modelCmd.AddCommand(modelListCmd)
	modelListCmd.Flags().BoolVarP(&modelVerbose, "verbose", "v", false, "Show detailed information")
	modelListCmd.Flags().StringVar(&modelCategory, "category", "", "Filter by category (compute, storage, network, security, data, custom)")
}

func runModelListCommand() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	discoverer := model.NewDiscoverer(cfg.SchemaPath)
	models, err := discoverer.ListModels()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to list models", err)
	}

	// Filter by category if specified
	if modelCategory != "" {
		var filtered []model.ModelInfo
		for _, m := range models {
			if m.Metadata.Category == modelCategory {
				filtered = append(filtered, m)
			}
		}
		models = filtered
	}

	if len(models) == 0 {
		fmt.Println("No models found.")
		if modelCategory != "" {
			fmt.Printf("\nNo models in category '%s'. Try: pudl model list\n", modelCategory)
		}
		return nil
	}

	fmt.Println("Available Models:")
	fmt.Println()

	for _, m := range models {
		actionCount := 0
		qualCount := 0
		for _, method := range m.Methods {
			switch method.Kind {
			case "qualification":
				qualCount++
			default:
				actionCount++
			}
		}

		inputCount := 0
		outputCount := 0
		for _, s := range m.Sockets {
			if s.Direction == "input" {
				inputCount++
			} else {
				outputCount++
			}
		}

		if modelVerbose {
			fmt.Printf("  %s\n", m.Name)
			fmt.Printf("    Description: %s\n", m.Metadata.Description)
			fmt.Printf("    Category:    %s\n", m.Metadata.Category)
			fmt.Printf("    Package:     %s\n", m.Package)
			fmt.Printf("    File:        %s\n", m.FilePath)
			fmt.Printf("    Methods:     %d actions, %d qualifications\n", actionCount, qualCount)
			fmt.Printf("    Sockets:     %d inputs, %d outputs\n", inputCount, outputCount)
			if m.Auth != nil {
				fmt.Printf("    Auth:        %s\n", m.Auth.Method)
			}
			fmt.Println()
		} else {
			authStr := ""
			if m.Auth != nil {
				authStr = fmt.Sprintf("  auth:%s", m.Auth.Method)
			}
			fmt.Printf("  %-50s [%s]  %d methods, %d sockets%s\n",
				m.Name, m.Metadata.Category, len(m.Methods), len(m.Sockets), authStr)
		}
	}

	fmt.Printf("\nTotal: %d models\n", len(models))
	return nil
}

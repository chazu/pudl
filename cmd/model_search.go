package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/model"
)

var modelSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search models by keyword",
	Long: `Search across model name, description, method names, socket names, and category.

The query is a case-insensitive substring match against all searchable fields.

Examples:
    pudl model search compute
    pudl model search instance
    pudl model search list`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelSearchCommand(args[0])
	},
}

func init() {
	modelCmd.AddCommand(modelSearchCmd)
}

func runModelSearchCommand(query string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	discoverer := model.NewDiscoverer(cfg.SchemaPath)
	models, err := discoverer.ListModels()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to list models", err)
	}

	query = strings.ToLower(query)
	var matches []model.ModelInfo

	for _, m := range models {
		if modelMatchesQuery(m, query) {
			matches = append(matches, m)
		}
	}

	if len(matches) == 0 {
		fmt.Printf("No models matching %q found.\n", query)
		return nil
	}

	fmt.Printf("Models matching %q:\n\n", query)

	for _, m := range matches {
		authStr := ""
		if m.Auth != nil {
			authStr = fmt.Sprintf("  auth:%s", m.Auth.Method)
		}
		fmt.Printf("  %-50s [%s]  %d methods, %d sockets%s\n",
			m.Name, m.Metadata.Category, len(m.Methods), len(m.Sockets), authStr)
		if m.Metadata.Description != "" {
			fmt.Printf("    %s\n", m.Metadata.Description)
		}
	}

	fmt.Printf("\nFound: %d models\n", len(matches))
	return nil
}

// modelMatchesQuery checks if any searchable field of the model contains the query.
func modelMatchesQuery(m model.ModelInfo, query string) bool {
	// Check model name and metadata
	if strings.Contains(strings.ToLower(m.Name), query) ||
		strings.Contains(strings.ToLower(m.Metadata.Name), query) ||
		strings.Contains(strings.ToLower(m.Metadata.Description), query) ||
		strings.Contains(strings.ToLower(m.Metadata.Category), query) {
		return true
	}

	// Check method names
	for name := range m.Methods {
		if strings.Contains(strings.ToLower(name), query) {
			return true
		}
	}

	// Check socket names
	for name := range m.Sockets {
		if strings.Contains(strings.ToLower(name), query) {
			return true
		}
	}

	return false
}

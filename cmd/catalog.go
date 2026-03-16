package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/validator"
)

// catalogCmd represents the catalog command
var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Display the schema catalog",
	Long: `List all registered schema types with their metadata.

The catalog is a central inventory of pudl's schema types, showing each
registered type along with its schema_type, resource_type, and description.

The catalog includes built-in types (pudl/core.#Item, pudl/core.#Collection)
and any user-defined types that include _pudl metadata.

Examples:
    pudl catalog              # List all registered types
    pudl catalog --verbose    # Show additional metadata fields`,
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)
		if err := runCatalogCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

var catalogVerbose bool

func init() {
	rootCmd.AddCommand(catalogCmd)
	catalogCmd.Flags().BoolVarP(&catalogVerbose, "verbose", "v", false, "Show additional metadata fields")
}

// catalogEntry holds the display info for a single catalog entry
type catalogEntry struct {
	Name         string
	SchemaType   string
	ResourceType string
	Description  string
	// Extended metadata (verbose only)
	IdentityFields []string
	TrackedFields  []string
	IsListType     bool
}

// runCatalogCommand loads all schemas and displays the catalog
func runCatalogCommand() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Load all CUE modules
	loader := validator.NewCUEModuleLoader(cfg.SchemaPath)
	modules, err := loader.LoadAllModules()
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed,
			"Failed to load CUE schemas", err)
	}

	// Collect all metadata into catalog entries
	allMetadata := loader.GetAllMetadata(modules)

	var entries []catalogEntry
	for name, meta := range allMetadata {
		entry := catalogEntry{
			Name:           name,
			SchemaType:     meta.SchemaType,
			ResourceType:   meta.ResourceType,
			IdentityFields: meta.IdentityFields,
			TrackedFields:  meta.TrackedFields,
			IsListType:     meta.IsListType,
		}
		// Provide defaults for display when metadata is sparse
		if entry.SchemaType == "" {
			entry.SchemaType = "custom"
		}
		if entry.ResourceType == "" {
			entry.ResourceType = "-"
		}
		entries = append(entries, entry)
	}

	// Sort entries by name for stable output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	if len(entries) == 0 {
		fmt.Println("No schemas found in the catalog.")
		fmt.Println()
		fmt.Println("Run 'pudl init' to set up bootstrap schemas.")
		return nil
	}

	fmt.Println("Schema Catalog:")
	fmt.Println()

	for _, e := range entries {
		fmt.Printf("  %-40s  type=%-12s  resource=%s\n", e.Name, e.SchemaType, e.ResourceType)

		if catalogVerbose {
			if len(e.IdentityFields) > 0 {
				fmt.Printf("    identity_fields: %v\n", e.IdentityFields)
			}
			if len(e.TrackedFields) > 0 {
				fmt.Printf("    tracked_fields:  %v\n", e.TrackedFields)
			}
			if e.IsListType {
				fmt.Printf("    list_type:       true\n")
			}
		}
	}

	fmt.Printf("\nTotal: %d registered types\n", len(entries))
	return nil
}

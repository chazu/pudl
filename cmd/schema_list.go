package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/schema"
)

// schemaListCmd represents the schema list command
var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available schemas",
	Long: `List all available schemas organized by package.

Schemas are displayed with their package, name, file size, and main definition.
Use --verbose for additional details including file paths and metadata information.

Filtering Options:
- --package: Show only schemas from a specific package

Examples:
    pudl schema list                    # List all schemas
    pudl schema list --package aws      # List only AWS schemas
    pudl schema list --verbose          # Show detailed information`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the schema list command and handle any errors
		if err := runSchemaListCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	schemaCmd.AddCommand(schemaListCmd)

	schemaListCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed information")
	schemaListCmd.Flags().StringVar(&schemaPackage, "package", "", "Filter by package name")

	schemaListCmd.RegisterFlagCompletionFunc("package", completeSchemaPackages)
}

// runSchemaListCommand contains the actual schema list logic with structured error handling
func runSchemaListCommand() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create schema manager
	manager := schema.NewManager(cfg.SchemaPath)

	if schemaPackage != "" {
		// List schemas in specific package
		return listSchemasInPackage(manager, schemaPackage)
	} else {
		// List all schemas
		return listAllSchemas(manager)
	}
}

// listAllSchemas lists all schemas organized by package
func listAllSchemas(manager *schema.Manager) error {
	schemas, err := manager.ListSchemas()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to list schemas", err)
	}

	if len(schemas) == 0 {
		fmt.Println("No schemas found.")
		fmt.Println()
		fmt.Println("💡 Add your first schema:")
		fmt.Println("   pudl schema add aws.ec2-instance my-schema.cue")
		return nil
	}

	fmt.Println("Available Schemas:")
	fmt.Println()

	totalSchemas := 0
	for packageName, packageSchemas := range schemas {
		fmt.Printf("📦 Package: %s (%d schemas)\n", packageName, len(packageSchemas))

		for _, schemaInfo := range packageSchemas {
			totalSchemas++
			if schemaVerbose {
				fmt.Printf("   ├─ %s\n", schemaInfo.Name)
				fmt.Printf("   │  Full name: %s\n", schemaInfo.FullName)
				fmt.Printf("   │  File: %s\n", schemaInfo.FilePath)
				fmt.Printf("   │  Size: %s\n", formatBytes(schemaInfo.Size))
				fmt.Printf("   │\n")
			} else {
				fmt.Printf("   ├─ %s\n", schemaInfo.Name)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d schemas in %d packages\n", totalSchemas, len(schemas))
	return nil
}

// listSchemasInPackage lists schemas in a specific package
func listSchemasInPackage(manager *schema.Manager, packageName string) error {
	schemas, err := manager.GetSchemasInPackage(packageName)
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem,
			fmt.Sprintf("Failed to list schemas in package '%s'", packageName), err)
	}

	if len(schemas) == 0 {
		fmt.Printf("No schemas found in package '%s'.\n", packageName)
		return nil
	}

	fmt.Printf("Schemas in package '%s':\n", packageName)
	fmt.Println()

	for _, schemaInfo := range schemas {
		if schemaVerbose {
			fmt.Printf("📄 %s\n", schemaInfo.Name)
			fmt.Printf("   Full name: %s\n", schemaInfo.FullName)
			fmt.Printf("   File: %s\n", schemaInfo.FilePath)
			fmt.Printf("   Size: %s\n", formatBytes(schemaInfo.Size))
			fmt.Println()
		} else {
			fmt.Printf("  %s\n", schemaInfo.Name)
		}
	}

	if !schemaVerbose {
		fmt.Printf("\nTotal: %d schemas\n", len(schemas))
	}
	return nil
}

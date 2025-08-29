package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/schema"
)

var (
	schemaVerbose bool
	schemaPackage string
)

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Manage CUE schemas for data validation",
	Long: `Manage CUE schemas used for data validation and organization in PUDL.

Schemas are organized by packages (aws, k8s, unknown, etc.) and stored in the
schema repository at ~/.pudl/schema/. Each schema is a CUE file that defines
the structure and validation rules for imported data.

Available subcommands:
- list: Show available schemas organized by package
- add:  Add a new schema file to the repository

Examples:
    pudl schema list                           # List all schemas
    pudl schema list --package aws             # List schemas in aws package
    pudl schema add aws.rds-instance my.cue    # Add new schema to aws package`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: show help
		cmd.Help()
	},
}

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
		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		// Create schema manager
		manager := schema.NewManager(cfg.SchemaPath)

		if schemaPackage != "" {
			// List schemas in specific package
			listSchemasInPackage(manager, schemaPackage)
		} else {
			// List all schemas
			listAllSchemas(manager)
		}
	},
}

// schemaAddCmd represents the schema add command
var schemaAddCmd = &cobra.Command{
	Use:   "add <package>.<name> <cue-file>",
	Short: "Add a new schema to the repository",
	Long: `Add a new CUE schema file to the schema repository.

The schema name should be in the format 'package.name' where:
- package: The schema package (aws, k8s, custom, etc.)
- name: The schema name within the package

The CUE file will be validated before adding to ensure:
- Valid CUE syntax
- Proper package declaration
- Required metadata fields (_identity, _tracked, _version)

The schema file will be copied to the appropriate package directory and
added to the git working directory (not automatically committed).

Examples:
    pudl schema add aws.rds-instance rds-schema.cue
    pudl schema add k8s.deployment my-deployment.cue
    pudl schema add custom.api-response api.cue`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		fullSchemaName := args[0]
		sourceFile := args[1]

		// Parse schema name
		packageName, schemaName, err := schema.ParseSchemaName(fullSchemaName)
		if err != nil {
			log.Fatalf("Invalid schema name: %v", err)
		}

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		// Create schema manager and validator
		manager := schema.NewManager(cfg.SchemaPath)
		validator := schema.NewValidator()

		// Validate the source file first
		fmt.Printf("Validating schema file: %s\n", sourceFile)
		result, err := validator.ValidateSchema(sourceFile)
		if err != nil {
			log.Fatalf("Failed to validate schema: %v", err)
		}

		// Check validation results
		if !result.Valid {
			fmt.Println("❌ Schema validation failed:")
			for _, error := range result.Errors {
				fmt.Printf("  - %s\n", error)
			}
			os.Exit(1)
		}

		// Show warnings if any
		if len(result.Warnings) > 0 {
			fmt.Println("⚠️  Validation warnings:")
			for _, warning := range result.Warnings {
				fmt.Printf("  - %s\n", warning)
			}
		}

		// Validate package consistency
		if result.PackageName != "" && result.PackageName != packageName {
			log.Fatalf("Package mismatch: schema declares package '%s' but adding to package '%s'", 
				result.PackageName, packageName)
		}

		// Check if schema already exists
		if manager.SchemaExists(packageName, schemaName) {
			log.Fatalf("Schema already exists: %s.%s", packageName, schemaName)
		}

		// Add the schema
		fmt.Printf("Adding schema: %s.%s\n", packageName, schemaName)
		if err := manager.AddSchema(packageName, schemaName, sourceFile); err != nil {
			log.Fatalf("Failed to add schema: %v", err)
		}

		fmt.Printf("✅ Schema added successfully: %s.%s\n", packageName, schemaName)
		
		// Show schema information
		if schemaInfo, err := manager.GetSchema(packageName, schemaName); err == nil {
			fmt.Printf("   Package: %s\n", schemaInfo.Package)
			fmt.Printf("   Name: %s\n", schemaInfo.Name)
			fmt.Printf("   File: %s\n", schemaInfo.FilePath)
			if schemaInfo.Definition != "" {
				fmt.Printf("   Definition: %s\n", schemaInfo.Definition)
			}
			if len(result.Definitions) > 0 {
				fmt.Printf("   Definitions: %s\n", strings.Join(result.Definitions, ", "))
			}
		}

		fmt.Println()
		fmt.Println("💡 Next steps:")
		fmt.Println("   - Review the schema: pudl schema list --package " + packageName)
		fmt.Println("   - Import data using this schema: pudl import --path <file> --schema " + fullSchemaName)
		fmt.Println("   - Commit schema changes: git -C ~/.pudl/schema add . && git -C ~/.pudl/schema commit -m \"Add " + fullSchemaName + " schema\"")
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)

	// Add subcommands
	schemaCmd.AddCommand(schemaListCmd)
	schemaCmd.AddCommand(schemaAddCmd)

	// Add flags
	schemaListCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed information")
	schemaListCmd.Flags().StringVar(&schemaPackage, "package", "", "Filter by package name")
}

// listAllSchemas lists all schemas organized by package
func listAllSchemas(manager *schema.Manager) {
	schemas, err := manager.ListSchemas()
	if err != nil {
		log.Fatalf("Failed to list schemas: %v", err)
	}

	if len(schemas) == 0 {
		fmt.Println("No schemas found.")
		fmt.Println()
		fmt.Println("💡 Add your first schema:")
		fmt.Println("   pudl schema add aws.ec2-instance my-schema.cue")
		return
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
				fmt.Printf("   │  File: %s\n", schemaInfo.FilePath)
				fmt.Printf("   │  Size: %s\n", formatBytes(schemaInfo.Size))
				if schemaInfo.Definition != "" {
					fmt.Printf("   │  Definition: %s\n", schemaInfo.Definition)
				}
				fmt.Printf("   │\n")
			} else {
				definitionInfo := ""
				if schemaInfo.Definition != "" {
					definitionInfo = fmt.Sprintf(" (%s)", schemaInfo.Definition)
				}
				fmt.Printf("   ├─ %s%s\n", schemaInfo.Name, definitionInfo)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d schemas in %d packages\n", totalSchemas, len(schemas))
}

// listSchemasInPackage lists schemas in a specific package
func listSchemasInPackage(manager *schema.Manager, packageName string) {
	schemas, err := manager.GetSchemasInPackage(packageName)
	if err != nil {
		log.Fatalf("Failed to list schemas in package '%s': %v", packageName, err)
	}

	if len(schemas) == 0 {
		fmt.Printf("No schemas found in package '%s'.\n", packageName)
		return
	}

	fmt.Printf("Schemas in package '%s':\n", packageName)
	fmt.Println()

	for _, schemaInfo := range schemas {
		if schemaVerbose {
			fmt.Printf("📄 %s\n", schemaInfo.Name)
			fmt.Printf("   File: %s\n", schemaInfo.FilePath)
			fmt.Printf("   Size: %s\n", formatBytes(schemaInfo.Size))
			if schemaInfo.Definition != "" {
				fmt.Printf("   Definition: %s\n", schemaInfo.Definition)
			}
			fmt.Println()
		} else {
			definitionInfo := ""
			if schemaInfo.Definition != "" {
				definitionInfo = fmt.Sprintf(" (%s)", schemaInfo.Definition)
			}
			fmt.Printf("  %s%s\n", schemaInfo.Name, definitionInfo)
		}
	}

	if !schemaVerbose {
		fmt.Printf("\nTotal: %d schemas\n", len(schemas))
	}
}



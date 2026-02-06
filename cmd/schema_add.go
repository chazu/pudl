package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/schema"
)

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
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the schema add command and handle any errors
		if err := runSchemaAddCommand(args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	schemaCmd.AddCommand(schemaAddCmd)
}

// runSchemaAddCommand contains the actual schema add logic with structured error handling
func runSchemaAddCommand(args []string) error {
	fullSchemaName := args[0]
	sourceFile := args[1]

	// Parse schema name
	packageName, schemaName, err := schema.ParseSchemaName(fullSchemaName)
	if err != nil {
		return errors.NewInputError("Invalid schema name format",
			"Use format: package.schema (e.g., aws.ec2-instance)")
	}

	// Check if source file exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		return errors.NewFileNotFoundError(sourceFile)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create schema manager and validator
	manager := schema.NewManager(cfg.SchemaPath)
	validator := schema.NewValidator()

	// Validate the source file first
	fmt.Printf("Validating schema file: %s\n", sourceFile)
	result, err := validator.ValidateSchema(sourceFile)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to validate schema", err)
	}

	// Check validation results
	if !result.Valid {
		return errors.NewCUESyntaxError(sourceFile, fmt.Errorf("validation failed: %v", result.Errors))
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
		return errors.NewInputError(
			fmt.Sprintf("Package mismatch: schema declares package '%s' but adding to package '%s'",
				result.PackageName, packageName),
			"Update the package declaration in the schema file",
			"Use the correct package name in the command")
	}

	// Check if schema already exists
	if manager.SchemaExists(packageName, schemaName) {
		return errors.NewInputError(
			fmt.Sprintf("Schema already exists: %s.%s", packageName, schemaName),
			"Use a different schema name",
			"Remove the existing schema first if you want to replace it")
	}

	// Add the schema
	fmt.Printf("Adding schema: %s.%s\n", packageName, schemaName)
	if err := manager.AddSchema(packageName, schemaName, sourceFile); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to add schema", err)
	}

	fmt.Printf("✅ Schema added successfully: %s.%s\n", packageName, schemaName)

	// Show definitions found in the added file
	if len(result.Definitions) > 0 {
		fmt.Printf("   Package: %s\n", packageName)
		fmt.Printf("   Definitions: %s\n", strings.Join(result.Definitions, ", "))
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Println("   - Review the schema: pudl schema list --package " + packageName)
	fmt.Println("   - Import data using this schema: pudl import --path <file> --schema " + fullSchemaName)
	fmt.Println("   - Commit schema changes: pudl schema commit -m \"Add " + fullSchemaName + " schema\"")

	return nil
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/schema"
	"github.com/chazu/pudl/internal/schemaname"
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
	RunE: pudlRunE(func(cmd *cobra.Command, args []string) error {
		return runSchemaAddCommand(args)
	}),
}

func init() {
	schemaCmd.AddCommand(schemaAddCmd)
	schemaAddCmd.ValidArgsFunction = completeSchemaNames
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
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create schema manager and validator
	manager := schema.NewManagerWithPaths(effectiveSchemaPaths(cfg)...)
	validator := schema.NewValidator()

	// Validate the source file first
	fmt.Fprintf(outw(), "Validating schema file: %s\n", sourceFile)
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
		fmt.Fprintln(outw(), "⚠️  Validation warnings:")
		for _, warning := range result.Warnings {
			fmt.Fprintf(outw(), "  - %s\n", warning)
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
			fmt.Sprintf("Schema file already exists: %s", filepath.Join(packageName, schemaName+".cue")),
			"Use a different schema name",
			"Remove the existing schema first if you want to replace it")
	}

	// Add the schema
	fmt.Fprintf(outw(), "Adding schema file: %s\n", filepath.Join(packageName, schemaName+".cue"))
	if err := manager.AddSchema(packageName, schemaName, sourceFile); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to add schema", err)
	}

	fmt.Fprintf(outw(), "✅ Schema file added successfully: %s\n", filepath.Join(effectiveSchemaPath(cfg), packageName, schemaName+".cue"))

	// Show definitions found in the added file
	var references []string
	for _, definition := range result.Definitions {
		references = append(references, schemaname.Format(packageName, definition))
	}
	if len(result.Definitions) > 0 {
		fmt.Fprintf(outw(), "   Package: %s\n", packageName)
		fmt.Fprintf(outw(), "   Definitions: %s\n", strings.Join(references, ", "))
	}

	fmt.Fprintln(outw())
	fmt.Fprintln(outw(), "💡 Next steps:")
	fmt.Fprintln(outw(), "   - Review the schema: pudl schema list --package "+packageName)
	for _, reference := range references {
		fmt.Fprintln(outw(), "   - Inspect the schema: pudl schema show "+reference)
	}
	if len(references) == 1 {
		fmt.Fprintln(outw(), "   - Import data using this schema: pudl import --path <file> --schema "+references[0])
	}
	fmt.Fprintln(outw(), "   - Stage the intended schema files and use git commit -m \"Add schema file "+filepath.Join(packageName, schemaName+".cue")+"\"")

	return nil
}

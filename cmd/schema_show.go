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

// schemaShowCmd represents the schema show command
var schemaShowCmd = &cobra.Command{
	Use:   "show <schema-name>",
	Short: "Display the contents of a schema",
	Long: `Display the contents of a schema definition.

The schema name can be specified in formats like:
  - aws/ec2.#Instance     (package.#Definition)
  - aws/ec2:#Instance     (package:#Definition)

Examples:
    pudl schema show aws/ec2.#Instance
    pudl schema show pudl/core.#Item
    pudl s show aws/ec2:#Instance`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeSchemaNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaShowCommand(args[0])
	},
}

func init() {
	schemaCmd.AddCommand(schemaShowCmd)
}

// runSchemaShowCommand displays the contents of a schema
func runSchemaShowCommand(schemaArg string) error {
	// Parse the schema argument - can be:
	// - aws/ec2.#Instance (package.#Definition using .)
	// - aws/ec2:#Instance (package:#Definition using :)
	var packagePath, definitionName string

	// First try :# separator
	if idx := strings.Index(schemaArg, ":#"); idx != -1 {
		packagePath = schemaArg[:idx]
		definitionName = schemaArg[idx+2:] // Skip :#
	} else if idx := strings.Index(schemaArg, ".#"); idx != -1 {
		// Then try .# separator
		packagePath = schemaArg[:idx]
		definitionName = schemaArg[idx+2:] // Skip .#
	} else {
		return errors.NewInputError(
			fmt.Sprintf("Invalid schema format: %s. Expected format: package/path.#Definition or package/path:#Definition", schemaArg))
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Use schema manager to find the schema
	manager := schema.NewManager(cfg.SchemaPath)
	schemaInfo, err := manager.GetSchema(packagePath, definitionName)
	if err != nil {
		return errors.NewFileNotFoundError(
			fmt.Sprintf("Schema not found: %s.#%s", packagePath, definitionName))
	}

	// Read the schema file content
	content, err := os.ReadFile(schemaInfo.FilePath)
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem,
			fmt.Sprintf("Failed to read schema file: %s", schemaInfo.FilePath), err)
	}

	// Display metadata
	fmt.Printf("📄 Schema: %s\n", schemaInfo.FullName)
	fmt.Printf("   Package: %s\n", schemaInfo.Package)
	fmt.Printf("   File: %s\n", schemaInfo.FilePath)
	fmt.Printf("   Size: %s\n", formatBytes(schemaInfo.Size))
	fmt.Println()
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Println()

	// Display the file content
	fmt.Print(string(content))

	// Ensure there's a trailing newline
	if len(content) > 0 && content[len(content)-1] != '\n' {
		fmt.Println()
	}

	return nil
}

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/schema"
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
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Use schema manager to find the schema
	manager := schema.NewManagerWithPaths(effectiveSchemaPaths(cfg)...)
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

	if jsonOutput {
		return printJSON(map[string]any{
			"schema":     schemaInfo.FullName,
			"package":    schemaInfo.Package,
			"file":       schemaInfo.FilePath,
			"size_bytes": schemaInfo.Size,
			"source":     string(content),
		})
	}

	// Display metadata
	fmt.Fprintf(outw(), "📄 Schema: %s\n", schemaInfo.FullName)
	fmt.Fprintf(outw(), "   Package: %s\n", schemaInfo.Package)
	fmt.Fprintf(outw(), "   File: %s\n", schemaInfo.FilePath)
	fmt.Fprintf(outw(), "   Size: %s\n", formatBytes(schemaInfo.Size))
	fmt.Fprintln(outw())
	fmt.Fprintln(outw(), "─────────────────────────────────────────────────────────")
	fmt.Fprintln(outw())

	// Display the file content
	fmt.Fprint(outw(), string(content))

	// Ensure there's a trailing newline
	if len(content) > 0 && content[len(content)-1] != '\n' {
		fmt.Fprintln(outw())
	}

	return nil
}

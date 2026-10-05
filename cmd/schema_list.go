package cmd

import (
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/schema"
	"github.com/chazu/pudl/internal/validator"
	"github.com/spf13/cobra"
)

var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List schema definitions and resource metadata",
	Long: `List schemas using full package.#Definition names accepted by 'pudl schema show'.
The listing includes resource types. Use --verbose for source paths, identity fields,
tracked fields, and collection metadata, or --json for the complete representation.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error { return runSchemaListCommand() },
}

type schemaListing struct {
	schema.SchemaInfo
	Metadata *validator.SchemaMetadata `json:"metadata,omitempty"`
}

func runSchemaListCommand() error {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return err
	}
	paths := effectiveSchemaPaths(cfg)
	manager := schema.NewManagerWithPaths(paths...)
	manager.SetBuiltInPackages(importer.BootstrapPackages())
	packages, err := manager.ListSchemas()
	if err != nil {
		return err
	}
	schemas, err := validator.NewChainValidator(paths...)
	if err != nil {
		return err
	}
	entries := make([]schemaListing, 0)
	for pkg, definitions := range packages {
		if schemaPackage != "" && pkg != schemaPackage {
			continue
		}
		for _, definition := range definitions {
			entry := schemaListing{SchemaInfo: definition}
			if metadata, found := schemas.GetSchemaMetadata(definition.FullName); found {
				entry.Metadata = &metadata
			}
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].FullName < entries[j].FullName })
	if jsonOutput {
		return GetOutputWriter().WriteJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(outw(), "No schemas found.")
		return nil
	}
	fmt.Fprintln(outw(), "Available Schemas:")
	for _, entry := range entries {
		builtIn := ""
		if entry.BuiltIn {
			builtIn = " [built-in]"
		}
		fmt.Fprintf(outw(), "  %s%s", entry.FullName, builtIn)
		if entry.Metadata != nil {
			fmt.Fprintf(outw(), "  type=%s resource=%s", entry.Metadata.SchemaType, entry.Metadata.ResourceType)
		}
		fmt.Fprintln(outw())
		if schemaVerbose {
			fmt.Fprintf(outw(), "    File: %s\n    Size: %s\n", entry.FilePath, formatBytes(entry.Size))
			if entry.Metadata != nil {
				fmt.Fprintf(outw(), "    identity_fields: %v\n    tracked_fields: %v\n    list_type: %t\n", entry.Metadata.IdentityFields, entry.Metadata.TrackedFields, entry.Metadata.IsListType)
			}
		}
	}
	fmt.Fprintf(outw(), "\nTotal: %d schemas\n", len(entries))
	return nil
}

func init() {
	schemaCmd.AddCommand(schemaListCmd)
	schemaListCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show paths and schema metadata")
	schemaListCmd.Flags().StringVar(&schemaPackage, "package", "", "Filter by package name")
	schemaListCmd.RegisterFlagCompletionFunc("package", completeSchemaPackages)
}

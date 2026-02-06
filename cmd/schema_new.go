package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/errors"
	"pudl/internal/inference"
	"pudl/internal/schemagen"
	"pudl/internal/ui"
)

var (
	// New command flags
	schemaNewFrom       string
	schemaNewPath       string
	schemaNewCollection bool
	schemaNewInfer      []string
	schemaNewForce      bool
)

// schemaNewCmd represents the schema new command
var schemaNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Generate a new schema from imported data",
	Long: `Generate a new CUE schema by analyzing data from a previously imported entry.

This command creates a new schema file based on the structure of imported data,
inferring field types, identifying likely identity fields, and generating
appropriate CUE type definitions.

The --from flag specifies the proquint ID of the imported data to analyze.
The --path flag specifies where to create the schema (package path and definition name).

If the path contains a # character, everything after it is used as the definition name.
Otherwise, the last path component is capitalized and used as the definition name.

When --from points to a collection entry:
- Without --collection: analyzes individual items to create an item schema
- With --collection: creates a schema for the collection structure itself

The --infer flag allows specifying type hints for specific fields, such as
marking a field as an enum type.

Examples:
    pudl schema new --from hugib-dubuf --path aws/ec2:#Instance
    pudl schema new --from govim-nupab --path aws/ec2:#Instance  # from collection
    pudl schema new --from govim-nupab --path aws/ec2:#InstanceCollection --collection
    pudl schema new --from hugib-dubuf --path aws/ec2:#Instance --infer State=enum`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaNewCommand()
	},
}

func init() {
	schemaCmd.AddCommand(schemaNewCmd)

	schemaNewCmd.Flags().StringVar(&schemaNewFrom, "from", "", "Proquint ID of the imported data to generate schema from (required)")
	schemaNewCmd.Flags().StringVar(&schemaNewPath, "path", "", "Schema path in format 'package/path:#Definition' (required)")
	schemaNewCmd.Flags().BoolVar(&schemaNewCollection, "collection", false, "Create a collection schema instead of item schema")
	schemaNewCmd.Flags().StringArrayVar(&schemaNewInfer, "infer", []string{}, "Field inference hints (e.g., State=enum)")
	schemaNewCmd.Flags().BoolVar(&schemaNewForce, "force", false, "Overwrite existing schema files")
	schemaNewCmd.MarkFlagRequired("from")
	schemaNewCmd.MarkFlagRequired("path")

	schemaNewCmd.RegisterFlagCompletionFunc("from", completeProquintIDs)
}

// runSchemaNewCommand runs the schema new command
func runSchemaNewCommand() error {
	// Parse the --path flag to extract package path and definition name
	packagePath, definitionName := parseSchemaPath(schemaNewPath)

	// Parse --infer flags into map
	inferHints := parseInferHints(schemaNewInfer)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Initialize database connection
	catalogDB, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	// Look up the entry by proquint ID
	entry, err := catalogDB.GetEntryByProquint(schemaNewFrom)
	if err != nil {
		return err
	}

	// Create the generator
	generator := schemagen.NewGenerator(cfg.SchemaPath)

	// Check if this is a collection entry with --collection flag (smart collection generation)
	if entry.CollectionType != nil && *entry.CollectionType == "collection" && schemaNewCollection {
		return runSmartCollectionGeneration(catalogDB, generator, entry, packagePath, definitionName, inferHints, cfg)
	}

	// Standard schema generation (item schema or legacy collection)
	var data interface{}
	if entry.CollectionType != nil && *entry.CollectionType == "collection" && !schemaNewCollection {
		// It's a collection and --collection is NOT set: get all items and merge their data
		items, err := catalogDB.GetCollectionItems(entry.ID)
		if err != nil {
			return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get collection items", err)
		}

		if len(items) == 0 {
			return errors.NewInputError("Collection has no items",
				"Use --collection flag to create a schema for the collection itself")
		}

		// Load data from each item and create an array for the generator
		var itemsData []interface{}
		for _, item := range items {
			itemData, err := loadJSONFile(item.StoredPath)
			if err != nil {
				return errors.WrapError(errors.ErrCodeFileSystem,
					fmt.Sprintf("Failed to load item data from %s", item.StoredPath), err)
			}
			itemsData = append(itemsData, itemData)
		}
		data = itemsData
	} else {
		// Either it's a single item, or it's a collection with --collection set
		data, err = loadJSONFile(entry.StoredPath)
		if err != nil {
			return errors.WrapError(errors.ErrCodeFileSystem,
				fmt.Sprintf("Failed to load data from %s", entry.StoredPath), err)
		}
	}

	// Build generation options
	opts := schemagen.GenerateOptions{
		FromID:         entry.ID,
		PackagePath:    packagePath,
		DefinitionName: definitionName,
		IsCollection:   schemaNewCollection,
		InferHints:     inferHints,
	}

	// Generate the schema
	result, err := generator.Generate(data, opts)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to generate schema", err)
	}

	// Write the schema file
	if err := generator.WriteSchema(result, result.Content, schemaNewForce); err != nil {
		// Check for schema exists error and provide better message
		if existsErr, ok := err.(*schemagen.SchemaExistsError); ok {
			return errors.NewInputError(
				fmt.Sprintf("Schema already exists: %s:#%s", existsErr.PackagePath, existsErr.DefinitionName),
				"Use --force to overwrite the existing schema",
			)
		}
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to write schema file", err)
	}

	// Check for JSON output
	output := GetOutputWriter()
	if output.Format == ui.OutputFormatJSON {
		jsonOutput := ui.SchemaNewOutput{
			Success:                true,
			FilePath:               result.FilePath,
			PackageName:            result.PackageName,
			DefinitionName:         result.DefinitionName,
			FieldCount:             result.FieldCount,
			InferredIdentityFields: result.InferredIdentityFields,
			IsCollection:           false,
		}
		return output.WriteJSON(jsonOutput)
	}

	// Print results (human-readable)
	fmt.Println("✅ Schema generated successfully!")
	fmt.Println()
	fmt.Printf("📄 File created: %s\n", result.FilePath)
	fmt.Printf("📦 Package: %s\n", result.PackageName)
	fmt.Printf("📋 Definition: #%s\n", result.DefinitionName)
	fmt.Printf("🔢 Fields: %d\n", result.FieldCount)

	if len(result.InferredIdentityFields) > 0 {
		fmt.Printf("🔑 Inferred identity fields: %s\n", strings.Join(result.InferredIdentityFields, ", "))
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Printf("   - Edit the schema: pudl schema edit %s:#%s\n", packagePath, result.DefinitionName)
	fmt.Printf("   - Commit changes: pudl schema commit -m \"Add %s schema\"\n", result.DefinitionName)

	return nil
}

// runSmartCollectionGeneration handles smart collection schema generation.
func runSmartCollectionGeneration(catalogDB *database.CatalogDB, generator *schemagen.Generator, entry *database.CatalogEntry, packagePath, definitionName string, inferHints map[string]string, cfg *config.Config) error {
	// Get collection items
	items, err := catalogDB.GetCollectionItems(entry.ID)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get collection items", err)
	}

	if len(items) == 0 {
		return errors.NewInputError("Collection has no items", "Cannot generate collection schema from empty collection")
	}

	// Load data from each item
	var itemsData []interface{}
	for _, item := range items {
		itemData, err := loadJSONFile(item.StoredPath)
		if err != nil {
			return errors.WrapError(errors.ErrCodeFileSystem,
				fmt.Sprintf("Failed to load item data from %s", item.StoredPath), err)
		}
		itemsData = append(itemsData, itemData)
	}

	// Initialize the schema inferrer
	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to initialize schema inferrer", err)
	}

	// Build collection generation options
	opts := schemagen.CollectionGenerateOptions{
		PackagePath:    packagePath,
		CollectionName: definitionName,
		InferHints:     inferHints,
	}

	// Generate the smart collection
	result, err := generator.GenerateSmartCollection(itemsData, opts, inferrer)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to generate collection schema", err)
	}

	// Track created item schemas for JSON output
	var createdItemSchemas []ui.SchemaNewItemOutput

	// Write any new item schemas first
	for _, itemSchema := range result.NewItemSchemas {
		if err := generator.WriteSchema(itemSchema, itemSchema.Content, schemaNewForce); err != nil {
			// Check for schema exists error and provide better message
			if existsErr, ok := err.(*schemagen.SchemaExistsError); ok {
				return errors.NewInputError(
					fmt.Sprintf("Item schema already exists: %s:#%s", existsErr.PackagePath, existsErr.DefinitionName),
					"Use --force to overwrite existing schemas",
				)
			}
			return errors.WrapError(errors.ErrCodeFileSystem,
				fmt.Sprintf("Failed to write item schema file: %s", itemSchema.FilePath), err)
		}
		createdItemSchemas = append(createdItemSchemas, ui.SchemaNewItemOutput{
			FilePath:       itemSchema.FilePath,
			DefinitionName: itemSchema.DefinitionName,
			FieldCount:     itemSchema.FieldCount,
		})
		// Only print if not JSON output
		output := GetOutputWriter()
		if output.Format != ui.OutputFormatJSON {
			fmt.Printf("📄 Created item schema: %s\n", itemSchema.FilePath)
		}
	}

	// Write the collection schema
	if err := generator.WriteSchema(result.CollectionSchema, result.CollectionSchema.Content, schemaNewForce); err != nil {
		// Check for schema exists error and provide better message
		if existsErr, ok := err.(*schemagen.SchemaExistsError); ok {
			return errors.NewInputError(
				fmt.Sprintf("Collection schema already exists: %s:#%s", existsErr.PackagePath, existsErr.DefinitionName),
				"Use --force to overwrite existing schemas",
			)
		}
		return errors.WrapError(errors.ErrCodeFileSystem,
			fmt.Sprintf("Failed to write collection schema file: %s", result.CollectionSchema.FilePath), err)
	}

	// Check for JSON output
	output := GetOutputWriter()
	if output.Format == ui.OutputFormatJSON {
		jsonOutput := ui.SchemaNewOutput{
			Success:            true,
			FilePath:           result.CollectionSchema.FilePath,
			PackageName:        result.CollectionSchema.PackageName,
			DefinitionName:     result.CollectionSchema.DefinitionName,
			FieldCount:         result.CollectionSchema.FieldCount,
			IsCollection:       true,
			NewItemSchemas:     createdItemSchemas,
			ExistingSchemaRefs: result.ExistingSchemaRefs,
		}
		return output.WriteJSON(jsonOutput)
	}

	// Print results (human-readable)
	fmt.Println()
	fmt.Println("✅ Collection schema generated successfully!")
	fmt.Println()
	fmt.Printf("📄 Collection schema: %s\n", result.CollectionSchema.FilePath)
	fmt.Printf("📦 Package: %s\n", result.CollectionSchema.PackageName)
	fmt.Printf("📋 Definition: #%s\n", result.CollectionSchema.DefinitionName)

	if len(result.ExistingSchemaRefs) > 0 {
		fmt.Println()
		fmt.Println("🔗 Reused existing schemas:")
		for _, ref := range result.ExistingSchemaRefs {
			fmt.Printf("   - %s\n", ref)
		}
	}

	if len(result.NewItemSchemas) > 0 {
		fmt.Println()
		fmt.Println("✨ Generated new item schemas:")
		for _, schema := range result.NewItemSchemas {
			fmt.Printf("   - #%s (%d fields)\n", schema.DefinitionName, schema.FieldCount)
		}
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Printf("   - Edit the collection schema: pudl schema edit %s:#%s\n", packagePath, definitionName)
	if len(result.NewItemSchemas) > 0 {
		fmt.Printf("   - Edit item schemas as needed\n")
	}
	fmt.Printf("   - Commit changes: pudl schema commit -m \"Add %s collection schema\"\n", definitionName)

	return nil
}

// parseSchemaPath parses the --path flag into package path and definition name
// e.g., "aws/ec2:#Instance" -> ("aws/ec2", "Instance")
// e.g., "aws/ec2" -> ("aws/ec2", "Ec2")
func parseSchemaPath(path string) (packagePath, definitionName string) {
	// Check if path contains # for explicit definition name
	if idx := strings.Index(path, ":#"); idx != -1 {
		packagePath = path[:idx]
		definitionName = path[idx+2:] // Skip :#
		return
	}

	// No explicit definition, capitalize the last path component
	packagePath = path
	parts := strings.Split(path, "/")
	lastPart := parts[len(parts)-1]
	definitionName = capitalizeFirst(lastPart)
	return
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// parseInferHints parses --infer flags into a map
// e.g., ["State=enum", "Type=enum"] -> {"State": "enum", "Type": "enum"}
func parseInferHints(hints []string) map[string]string {
	result := make(map[string]string)
	for _, hint := range hints {
		parts := strings.SplitN(hint, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// loadJSONFile loads and parses a JSON file
func loadJSONFile(path string) (interface{}, error) {
	fileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var data interface{}
	if err := json.Unmarshal(fileData, &data); err != nil {
		return nil, err
	}

	return data, nil
}

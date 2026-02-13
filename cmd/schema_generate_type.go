package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/schemagen"
	"pudl/internal/typepattern"
	"pudl/internal/ui"
)

var (
	// generate-type command flags
	generateTypeKind       string
	generateTypeAPIVersion string
	generateTypeEcosystem  string
	generateTypeType       string
	generateTypeForce      bool
	generateTypeDryRun     bool
)

// schemaGenerateTypeCmd represents the schema generate-type command
var schemaGenerateTypeCmd = &cobra.Command{
	Use:   "generate-type",
	Short: "Generate a schema from a detected type",
	Long: `Generate a CUE schema for a known type from the type registry.

This command allows you to manually generate schemas for well-known types
like Kubernetes resources, without needing sample data. The schema will
import and extend the canonical type definition.

For Kubernetes types, use --kind and --api-version:
    pudl schema generate-type --kind Job --api-version batch/v1
    pudl schema generate-type --kind Deployment --api-version apps/v1

For other ecosystems, use --ecosystem and --type:
    pudl schema generate-type --ecosystem aws --type ec2:Instance

Flags:
    --kind         K8s resource kind (e.g., Job, Deployment)
    --api-version  K8s API version (e.g., batch/v1, apps/v1)
    --ecosystem    Ecosystem name (kubernetes, aws, gitlab) - defaults to kubernetes
    --type         Generic type identifier for non-K8s (e.g., ec2:Instance)
    --force        Overwrite existing schema
    --dry-run      Print generated schema without saving`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaGenerateTypeCommand()
	},
}

func init() {
	schemaCmd.AddCommand(schemaGenerateTypeCmd)

	schemaGenerateTypeCmd.Flags().StringVar(&generateTypeKind, "kind", "", "Kubernetes resource kind (e.g., Job, Deployment)")
	schemaGenerateTypeCmd.Flags().StringVar(&generateTypeAPIVersion, "api-version", "", "Kubernetes API version (e.g., batch/v1, apps/v1)")
	schemaGenerateTypeCmd.Flags().StringVar(&generateTypeEcosystem, "ecosystem", "kubernetes", "Ecosystem name (kubernetes, aws, gitlab)")
	schemaGenerateTypeCmd.Flags().StringVar(&generateTypeType, "type", "", "Generic type identifier for non-K8s (e.g., ec2:Instance)")
	schemaGenerateTypeCmd.Flags().BoolVar(&generateTypeForce, "force", false, "Overwrite existing schema")
	schemaGenerateTypeCmd.Flags().BoolVar(&generateTypeDryRun, "dry-run", false, "Print generated schema without saving")
}

// runSchemaGenerateTypeCommand runs the schema generate-type command
func runSchemaGenerateTypeCommand() error {
	// Validate flags - need either kind+api-version or ecosystem+type
	if generateTypeKind != "" && generateTypeAPIVersion != "" {
		return generateKubernetesType()
	}
	if generateTypeType != "" {
		return generateGenericType()
	}

	// If kind is provided but not api-version, or vice versa
	if generateTypeKind != "" || generateTypeAPIVersion != "" {
		return errors.NewInputError(
			"Both --kind and --api-version are required for Kubernetes types",
			"Example: pudl schema generate-type --kind Job --api-version batch/v1",
		)
	}

	return errors.NewInputError(
		"Must specify either --kind/--api-version for Kubernetes, or --type for other ecosystems",
		"Example: pudl schema generate-type --kind Job --api-version batch/v1",
		"Example: pudl schema generate-type --ecosystem aws --type ec2:Instance",
	)
}

// generateKubernetesType generates a schema for a Kubernetes type
func generateKubernetesType() error {
	// Build the detected type
	detected := typepattern.BuildKubernetesDetectedType(generateTypeKind, generateTypeAPIVersion)

	// Check if this is a known type with import path
	if detected.ImportPath == "" {
		knownTypes := typepattern.GetKnownKubernetesTypes()
		return errors.NewInputError(
			fmt.Sprintf("Unknown Kubernetes type: %s:%s", generateTypeAPIVersion, generateTypeKind),
			"Known types: "+strings.Join(knownTypes, ", "),
		)
	}

	return generateSchemaFromDetectedType(detected)
}

// generateGenericType generates a schema for a non-Kubernetes type
func generateGenericType() error {
	// For non-Kubernetes ecosystems, we need to look up the pattern
	// Currently, AWS and GitLab patterns don't have external CUE schemas
	return errors.NewInputError(
		fmt.Sprintf("Ecosystem '%s' does not have canonical CUE schemas available", generateTypeEcosystem),
		"Currently only Kubernetes types with --kind/--api-version are supported",
		"AWS and GitLab schemas must be generated from sample data using 'pudl schema new'",
	)
}

// generateSchemaFromDetectedType generates and optionally saves a schema
func generateSchemaFromDetectedType(detected *typepattern.DetectedType) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create the generator
	generator := schemagen.NewGenerator(cfg.SchemaPath)

	// Generate the schema (no sample data needed for import-based schemas)
	result, err := generator.GenerateFromDetectedType(detected, nil)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to generate schema", err)
	}

	// Dry-run: just print the content
	if generateTypeDryRun {
		fmt.Println(result.Content)
		return nil
	}

	// Write the schema file
	// Use syntax-only validation for import-based schemas since dependencies
	// won't be available until after cue mod tidy runs
	var writeErr error
	if detected.ImportPath != "" {
		writeErr = generator.WriteSchemaWithSyntaxCheck(result, result.Content, generateTypeForce)
	} else {
		writeErr = generator.WriteSchema(result, result.Content, generateTypeForce)
	}
	if writeErr != nil {
		if existsErr, ok := writeErr.(*schemagen.SchemaExistsError); ok {
			return errors.NewInputError(
				fmt.Sprintf("Schema already exists: %s", existsErr.FilePath),
				"Use --force to overwrite the existing schema",
			)
		}
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to write schema file", writeErr)
	}

	// Try to run cue mod tidy to fetch dependencies
	runCueModTidy(cfg.SchemaPath)

	// Check for JSON output
	output := GetOutputWriter()
	if output.Format == ui.OutputFormatJSON {
		jsonOutput := ui.SchemaNewOutput{
			Success:        true,
			FilePath:       result.FilePath,
			PackageName:    result.PackageName,
			DefinitionName: result.DefinitionName,
			FieldCount:     result.FieldCount,
		}
		return output.WriteJSON(jsonOutput)
	}

	// Print human-readable results
	fmt.Println("✅ Schema generated successfully!")
	fmt.Println()
	fmt.Printf("📄 File created: %s\n", result.FilePath)
	fmt.Printf("📦 Package: %s\n", result.PackageName)
	fmt.Printf("📋 Definition: #%s\n", result.DefinitionName)
	if detected.ImportPath != "" {
		fmt.Printf("🔗 Extends: %s.#%s\n", detected.ImportPath, detected.Definition)
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Printf("   - Review the schema: pudl schema show %s:#%s\n", result.PackageName, result.DefinitionName)
	fmt.Printf("   - Commit changes: pudl schema commit -m \"Add %s schema\"\n", result.DefinitionName)

	return nil
}

// runCueModTidy attempts to run cue mod tidy to fetch dependencies
func runCueModTidy(schemaPath string) {
	// Check if CUE is available
	if _, err := exec.LookPath("cue"); err != nil {
		return
	}

	// Check if module.cue exists
	modulePath := filepath.Join(schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return
	}

	// Run cue mod tidy silently
	cmd := exec.Command("cue", "mod", "tidy")
	cmd.Dir = schemaPath
	_ = cmd.Run() // Ignore errors - this is best effort
}

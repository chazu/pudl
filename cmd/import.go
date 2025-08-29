package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/importer"
	"pudl/internal/validator"
)

var (
	importSchema string
	importOrigin string
)

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:   "import --path <file>",
	Short: "Import data into PUDL data lake",
	Long: `Import data from files into the PUDL data lake with automatic format detection
and schema assignment.

This command imports data from various formats (JSON, YAML, CSV) and stores it
in the PUDL data lake with full metadata tracking. The data is stored in raw
format with timestamp-based naming and metadata that includes schema assignment
via the Zygomys rule engine.

Data Storage:
- Raw data: ~/.pudl/data/raw/YYYY/MM/DD/YYYYMMDD_HHMMSS_origin.ext
- Metadata: ~/.pudl/data/metadata/YYYYMMDD_HHMMSS_origin.ext.meta
- Catalog: ~/.pudl/data/catalog/ (inventory, schema assignments, etc.)

Schema Assignment:
- Manual schema specification with --schema flag (cascading validation)
- Automatic schema detection using rule engine (fallback)
- Cascading validation: policy → base → generic → catchall
- Never rejects data - always finds appropriate schema

Example usage:
    pudl import --path data.json
    pudl import --path aws-instances.json --schema aws.compliant-ec2
    pudl import --path k8s-pods.yaml --schema k8s.pod --origin k8s-get-pods`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get the file path from --path flag
		filePath, err := cmd.Flags().GetString("path")
		if err != nil {
			log.Fatalf("Error getting path flag: %v", err)
		}

		if filePath == "" {
			log.Fatal("--path flag is required")
		}

		// Validate file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.Fatalf("File %s does not exist", filePath)
		}

		// Get absolute path
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			log.Fatalf("Failed to get absolute path: %v", err)
		}

		// Load configuration to get data directory
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		// Create importer and validator
		imp := importer.New(cfg.DataPath, cfg.SchemaPath)

		var cascadeValidator *validator.CascadeValidator
		if importSchema != "" {
			// Create cascade validator for manual schema specification
			cv, err := validator.NewCascadeValidator(cfg.SchemaPath)
			if err != nil {
				log.Fatalf("Failed to create cascade validator: %v", err)
			}
			cascadeValidator = cv

			// Resolve schema name
			resolvedSchema, err := cv.ResolveSchemaName(importSchema)
			if err != nil {
				log.Fatalf("Failed to resolve schema name '%s': %v", importSchema, err)
			}
			importSchema = resolvedSchema
		}

		// Set up import options
		opts := importer.ImportOptions{
			SourcePath:        absPath,
			Origin:           importOrigin, // Will be auto-detected if empty
			ManualSchema:     importSchema,
			CascadeValidator: cascadeValidator,
		}

		// Perform the import
		result, err := imp.ImportFile(opts)
		if err != nil {
			log.Fatalf("Failed to import file: %v", err)
		}

		// Display results
		displayImportResults(result)
	},
}

func init() {
	rootCmd.AddCommand(importCmd)

	// Add flags
	importCmd.Flags().StringP("path", "p", "", "Path to file to import (required)")
	importCmd.Flags().StringVar(&importOrigin, "origin", "", "Override origin detection (optional)")
	importCmd.Flags().StringVar(&importSchema, "schema", "", "Specify schema for validation (e.g., aws.compliant-ec2)")

	// Mark path as required
	importCmd.MarkFlagRequired("path")
}

// displayImportResults shows the results of data import with cascading validation info
func displayImportResults(result *importer.ImportResult) {
	fmt.Printf("✅ Successfully imported data!\n")
	fmt.Printf("   Source: %s\n", result.SourcePath)
	fmt.Printf("   Stored as: %s\n", result.StoredPath)
	fmt.Printf("   Format: %s\n", result.DetectedFormat)
	fmt.Printf("   Origin: %s\n", result.DetectedOrigin)

	// Show validation results if available
	if result.ValidationResult != nil {
		vr := result.ValidationResult
		fmt.Printf("   %s\n", vr.GetSummary())

		if vr.IntendedSchema != "" && vr.IntendedSchema != vr.AssignedSchema {
			fmt.Printf("   🎯 Intended Schema: %s\n", vr.IntendedSchema)
		}
		fmt.Printf("   📋 Assigned Schema: %s\n", vr.AssignedSchema)

		// Show compliance status
		complianceStatus := vr.GetComplianceStatus()
		switch complianceStatus {
		case "compliant":
			fmt.Printf("   ✅ Compliance: COMPLIANT\n")
		case "non-compliant":
			fmt.Printf("   ⚠️  Compliance: NON-COMPLIANT (marked as outlier)\n")
		case "partial":
			fmt.Printf("   🔄 Compliance: PARTIAL (fell back to base schema)\n")
		case "unknown":
			fmt.Printf("   ❓ Compliance: UNKNOWN (assigned to catchall)\n")
		}

		// Show error count if any
		if vr.HasErrors() {
			fmt.Printf("   ❌ Validation Issues: %d (see details with 'pudl show %s --validation')\n",
				vr.GetErrorCount(), result.ID)
		}
	} else {
		// Legacy display for auto-assigned schemas
		fmt.Printf("   Schema: %s\n", result.AssignedSchema)
		if result.SchemaConfidence < 0.8 {
			fmt.Printf("   ⚠️  Low schema confidence (%.2f) - data assigned to catchall\n", result.SchemaConfidence)
		}
	}

	fmt.Printf("   Records: %d\n", result.RecordCount)
	fmt.Printf("   Size: %d bytes\n", result.SizeBytes)

	// Show next steps
	if result.ValidationResult != nil && result.ValidationResult.IsNonCompliant() {
		fmt.Println()
		fmt.Println("💡 Next steps:")
		fmt.Println("   - Review compliance issues: pudl show " + result.ID + " --validation")
		fmt.Println("   - List similar outliers: pudl list --schema " + result.ValidationResult.AssignedSchema + " --compliance non-compliant")
	}
}

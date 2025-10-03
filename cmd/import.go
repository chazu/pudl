package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/importer"
	"pudl/internal/streaming"
	"pudl/internal/validator"
)

var (
	importSchema      string
	importOrigin      string
	useStreaming      bool
	streamingMemoryMB int
	streamingChunkMB  float64
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

Streaming Support:
- Use --streaming for large files (>100MB recommended)
- Configure memory limits with --streaming-memory (default: 100MB)
- Adjust chunk size with --streaming-chunk-size (default: 0.016MB)
- Enables processing of files larger than available RAM

Example usage:
    pudl import --path data.json
    pudl import --path aws-instances.json --schema aws.compliant-ec2
    pudl import --path k8s-pods.yaml --schema k8s.pod --origin k8s-get-pods
    pudl import --path large-dataset.json --streaming --streaming-memory 200`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true) // Exit on non-recoverable errors

		// Run the import command and handle any errors
		if err := runImportCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},

}

// runImportCommand contains the actual import logic with structured error handling
func runImportCommand(cmd *cobra.Command, args []string) error {
	// Get the file path from --path flag
	filePath, err := cmd.Flags().GetString("path")
	if err != nil {
		return errors.WrapError(errors.ErrCodeInvalidInput, "Error getting path flag", err)
	}

	if filePath == "" {
		return errors.NewMissingRequiredError("path")
	}

	// Validate file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError(filePath)
	}

	// Get absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to get absolute path", err)
	}

	// Load configuration to get data directory
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create importer and validator
	imp, err := importer.New(cfg.DataPath, cfg.SchemaPath, config.GetPudlDir())
	if err != nil {
		return errors.NewSystemError("Failed to initialize importer", err)
	}
	defer imp.Close()

	var cascadeValidator *validator.CascadeValidator
	if importSchema != "" {
		// Create cascade validator for manual schema specification
		cv, err := validator.NewCascadeValidator(cfg.SchemaPath)
		if err != nil {
			return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to create cascade validator", err)
		}
		cascadeValidator = cv

		// Resolve schema name
		resolvedSchema, err := cv.ResolveSchemaName(importSchema)
		if err != nil {
			return errors.NewSchemaNotFoundError(importSchema, nil)
		}
		importSchema = resolvedSchema
	}

	// Configure streaming if enabled
	var streamingConfig *streaming.StreamingConfig
	if useStreaming {
		streamingConfig = streaming.DefaultStreamingConfig()

		// Apply user-specified memory limit
		if streamingMemoryMB > 0 {
			streamingConfig.MaxMemoryMB = streamingMemoryMB
		}

		// Check file size to determine appropriate chunk sizes
		fileInfo, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("failed to get file info: %w", err)
		}
		fileSize := fileInfo.Size()

		// For small files (< 10KB), use very small chunk sizes
		// For larger files, use user-specified or default chunk sizes
		if fileSize < 10*1024 {
			// Small file: use tiny chunks to ensure proper chunking
			streamingConfig.MinChunkSize = 64     // 64 bytes minimum
			streamingConfig.AvgChunkSize = 256    // 256 bytes average
			streamingConfig.MaxChunkSize = 1024   // 1KB maximum
		} else {
			// Large file: use user-specified or default chunk sizes
			chunkBytes := int(streamingChunkMB * 1024 * 1024)
			streamingConfig.AvgChunkSize = chunkBytes
			streamingConfig.MinChunkSize = chunkBytes / 4  // 25% of avg
			streamingConfig.MaxChunkSize = chunkBytes * 4  // 400% of avg
		}
	}

	// Set up import options
	opts := importer.ImportOptions{
		SourcePath:        absPath,
		Origin:           importOrigin, // Will be auto-detected if empty
		ManualSchema:     importSchema,
		CascadeValidator: cascadeValidator,
		UseStreaming:     useStreaming,
		StreamingConfig:  streamingConfig,
	}

	// Perform the import
	result, err := imp.ImportFile(opts)
	if err != nil {
		return errors.WrapError(errors.ErrCodeParsingFailed, "Failed to import file", err)
	}

	// Display results
	displayImportResults(result)
	return nil
}

func init() {
	rootCmd.AddCommand(importCmd)

	// Add flags
	importCmd.Flags().StringP("path", "p", "", "Path to file to import (required)")
	importCmd.Flags().StringVar(&importOrigin, "origin", "", "Override origin detection (optional)")
	importCmd.Flags().StringVar(&importSchema, "schema", "", "Specify schema for validation (e.g., aws.compliant-ec2)")

	// Streaming options
	importCmd.Flags().BoolVar(&useStreaming, "streaming", false, "Use streaming parser for large files")
	importCmd.Flags().IntVar(&streamingMemoryMB, "streaming-memory", 100, "Memory limit for streaming parser (MB)")
	importCmd.Flags().Float64Var(&streamingChunkMB, "streaming-chunk-size", 0.016, "Average chunk size for streaming parser (MB)")

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

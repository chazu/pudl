package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	streamingMemoryMB int
	streamingChunkMB  float64
	useZygomys        bool
)

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:   "import --path <file-or-pattern>",
	Short: "Import data into PUDL data lake",
	Long: `Import data from files into the PUDL data lake with automatic format detection
and schema assignment.

This command imports data from various formats (JSON, YAML, CSV) and stores it
in the PUDL data lake with full metadata tracking. The data is stored in raw
format with timestamp-based naming and metadata that includes schema assignment
via the Zygomys rule engine.

The --path flag supports both single files and wildcard patterns for batch imports:
- Single file: --path data.json
- Wildcard patterns: --path *.json, --path data/*.yaml, --path logs/2024-*.json

Data Storage:
- Raw data: ~/.pudl/data/raw/YYYY/MM/DD/YYYYMMDD_HHMMSS_origin.ext
- Metadata: ~/.pudl/data/metadata/YYYYMMDD_HHMMSS_origin.ext.meta
- Catalog: ~/.pudl/data/catalog/ (inventory, schema assignments, etc.)

Schema Assignment:
- Manual schema specification with --schema flag (cascading validation)
- Automatic schema detection using rule engine (fallback)
- Cascading validation: policy → base → generic → catchall
- Never rejects data - always finds appropriate schema

Rule Engine Options:
- Default: Uses legacy hard-coded rules for schema assignment
- --use-zygomys: Override to use Zygomys Lisp rule engine for this import

Streaming Processing:
- All imports use streaming for optimal performance and memory usage
- Configure memory limits with --streaming-memory (default: 100MB)
- Adjust chunk size with --streaming-chunk-size (default: 0.016MB)
- Handles files of any size efficiently

Example usage:
    # Single file import
    pudl import --path data.json
    pudl import --path aws-instances.json --schema aws.compliant-ec2
    pudl import --path k8s-pods.yaml --schema k8s.pod --origin k8s-get-pods

    # Wildcard batch import
    pudl import --path *.json
    pudl import --path data/*.yaml
    pudl import --path logs/2024-01-*.json

    # Advanced options
    pudl import --path large-dataset.json --streaming-memory 200
    pudl import --path data.json --use-zygomys`,
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

	// Resolve file paths (handles both single files and wildcard patterns)
	filePaths, err := resolveFilePaths(filePath)
	if err != nil {
		return err
	}

	if len(filePaths) == 0 {
		return errors.NewFileNotFoundError(filePath + " (no files matched pattern)")
	}

	// If multiple files, perform batch import
	if len(filePaths) > 1 {
		return runBatchImport(cmd, filePaths)
	}

	// Single file import (existing logic)
	absPath := filePaths[0]

	// Load configuration to get data directory
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create enhanced importer with friendly ID support
	imp, err := importer.NewEnhancedImporter(cfg.DataPath, cfg.SchemaPath, config.GetPudlDir())
	if err != nil {
		return errors.NewSystemError("Failed to initialize enhanced importer", err)
	}
	defer imp.Close()

	// Override rule engine if --use-zygomys flag is set
	if useZygomys {
		if err := imp.OverrideRuleEngine("zygomys"); err != nil {
			return errors.NewSystemError("Failed to override rule engine to Zygomys", err)
		}
	}

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

	// Configure streaming (always enabled for optimal performance)
	streamingConfig := streaming.DefaultStreamingConfig()

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
		streamingConfig.MinChunkSize = 64   // 64 bytes minimum
		streamingConfig.AvgChunkSize = 256  // 256 bytes average
		streamingConfig.MaxChunkSize = 1024 // 1KB maximum
	} else {
		// Large file: use user-specified or default chunk sizes
		chunkBytes := int(streamingChunkMB * 1024 * 1024)
		streamingConfig.AvgChunkSize = chunkBytes
		streamingConfig.MinChunkSize = chunkBytes / 4 // 25% of avg
		streamingConfig.MaxChunkSize = chunkBytes * 4 // 400% of avg
	}

	// Set up import options
	opts := importer.ImportOptions{
		SourcePath:       absPath,
		Origin:           importOrigin, // Will be auto-detected if empty
		ManualSchema:     importSchema,
		CascadeValidator: cascadeValidator,
		UseStreaming:     true, // Always use streaming for optimal performance
		StreamingConfig:  streamingConfig,
	}

	// Perform the import with friendly IDs
	result, err := imp.ImportFileWithFriendlyIDs(opts)
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
	importCmd.Flags().StringP("path", "p", "", "Path to file or wildcard pattern to import (required)")
	importCmd.Flags().StringVar(&importOrigin, "origin", "", "Override origin detection (optional)")
	importCmd.Flags().StringVar(&importSchema, "schema", "", "Specify schema for validation (e.g., aws.compliant-ec2)")

	// Streaming options
	importCmd.Flags().IntVar(&streamingMemoryMB, "streaming-memory", 100, "Memory limit for streaming parser (MB)")
	importCmd.Flags().Float64Var(&streamingChunkMB, "streaming-chunk-size", 0.016, "Average chunk size for streaming parser (MB)")

	// Rule engine options
	importCmd.Flags().BoolVar(&useZygomys, "use-zygomys", false, "Use Zygomys Lisp rule engine instead of legacy rules")

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
		// Display auto-assigned schema
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

// resolveFilePaths resolves a file path that may contain wildcards to a list of actual file paths
func resolveFilePaths(pathPattern string) ([]string, error) {
	// Check if the path contains wildcard characters
	if !containsWildcard(pathPattern) {
		// Single file path - validate it exists
		if _, err := os.Stat(pathPattern); os.IsNotExist(err) {
			return nil, errors.NewFileNotFoundError(pathPattern)
		}

		// Get absolute path
		absPath, err := filepath.Abs(pathPattern)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeFileSystem, "Failed to get absolute path", err)
		}

		return []string{absPath}, nil
	}

	// Wildcard pattern - use filepath.Glob to resolve
	matches, err := filepath.Glob(pathPattern)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "Invalid wildcard pattern", err)
	}

	// Convert to absolute paths and filter out directories
	var filePaths []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue // Skip files that can't be accessed
		}

		// Only include regular files, not directories
		if info.Mode().IsRegular() {
			absPath, err := filepath.Abs(match)
			if err != nil {
				continue // Skip files where we can't get absolute path
			}
			filePaths = append(filePaths, absPath)
		}
	}

	return filePaths, nil
}

// containsWildcard checks if a path contains wildcard characters
func containsWildcard(path string) bool {
	return strings.ContainsAny(path, "*?[]")
}

// runBatchImport handles importing multiple files and provides summary output
func runBatchImport(cmd *cobra.Command, filePaths []string) error {
	fmt.Printf("🔄 Importing %d files...\n\n", len(filePaths))

	// Load configuration to get data directory
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create enhanced importer with friendly ID support
	imp, err := importer.NewEnhancedImporter(cfg.DataPath, cfg.SchemaPath, config.GetPudlDir())
	if err != nil {
		return errors.NewSystemError("Failed to initialize enhanced importer", err)
	}
	defer imp.Close()

	// Override rule engine if --use-zygomys flag is set
	if useZygomys {
		if err := imp.OverrideRuleEngine("zygomys"); err != nil {
			return errors.NewSystemError("Failed to override rule engine to Zygomys", err)
		}
	}

	// Set up cascade validator if schema is specified
	var cascadeValidator *validator.CascadeValidator
	if importSchema != "" {
		cascadeValidator, err = validator.NewCascadeValidator(cfg.SchemaPath)
		if err != nil {
			return errors.NewSystemError("Failed to initialize cascade validator", err)
		}
	}

	// Set up streaming configuration
	streamingConfig := &streaming.StreamingConfig{
		ChunkAlgorithm: "fastcdc",
		MinChunkSize:   4096,
		MaxChunkSize:   65536,
		AvgChunkSize:   int(streamingChunkMB * 1024 * 1024), // Convert MB to bytes
		MaxMemoryMB:    streamingMemoryMB,
		BufferSize:     1048576, // 1MB
		ErrorTolerance: 0.1,
		SkipMalformed:  true,
		SampleSize:     100,
		Confidence:     0.8,
		ReportEveryMB:  1,
		MaxConcurrency: 0,
	}

	// Track results and errors
	var results []*importer.ImportResult
	var importErrors []error
	successCount := 0
	totalRecords := 0
	totalSize := int64(0)

	// Import each file
	for i, filePath := range filePaths {
		fmt.Printf("📁 [%d/%d] Importing: %s\n", i+1, len(filePaths), filepath.Base(filePath))

		// Set up import options for this file
		opts := importer.ImportOptions{
			SourcePath:       filePath,
			Origin:           importOrigin, // Will be auto-detected if empty
			ManualSchema:     importSchema,
			CascadeValidator: cascadeValidator,
			UseStreaming:     true, // Always use streaming for optimal performance
			StreamingConfig:  streamingConfig,
		}

		// Perform the import
		result, err := imp.ImportFileWithFriendlyIDs(opts)
		if err != nil {
			importErrors = append(importErrors, fmt.Errorf("failed to import %s: %w", filepath.Base(filePath), err))
			fmt.Printf("   ❌ Failed: %v\n", err)
			continue
		}

		// Track successful import
		results = append(results, result)
		successCount++
		totalRecords += result.RecordCount
		totalSize += result.SizeBytes

		fmt.Printf("   ✅ Success: %s (ID: %s, Records: %d)\n", result.DetectedFormat, result.ID, result.RecordCount)
	}

	// Display summary
	displayBatchImportSummary(successCount, len(filePaths), totalRecords, totalSize, importErrors)

	// If there were any errors, return the first one
	if len(importErrors) > 0 {
		return importErrors[0]
	}

	return nil
}

// displayBatchImportSummary shows a summary of batch import results
func displayBatchImportSummary(successCount, totalCount, totalRecords int, totalSize int64, importErrors []error) {
	fmt.Println()
	fmt.Printf("📊 Batch Import Summary\n")
	fmt.Printf("   Files processed: %d/%d\n", successCount, totalCount)
	fmt.Printf("   Total records: %d\n", totalRecords)
	fmt.Printf("   Total size: %d bytes\n", totalSize)

	if len(importErrors) > 0 {
		fmt.Printf("   Errors: %d\n", len(importErrors))
		fmt.Println()
		fmt.Println("❌ Import Errors:")
		for _, err := range importErrors {
			fmt.Printf("   - %v\n", err)
		}
	}

	if successCount > 0 {
		fmt.Println()
		fmt.Println("✅ Batch import completed!")
		if successCount < totalCount {
			fmt.Printf("   %d files imported successfully, %d failed\n", successCount, totalCount-successCount)
		} else {
			fmt.Printf("   All %d files imported successfully\n", successCount)
		}
	}
}

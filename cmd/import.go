package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/inference"
)

var (
	importSchema      string
	importOrigin      string
	importFormat      string
	streamingMemoryMB int
	streamingChunkMB  float64
)

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:   "import --path <file-or-pattern>",
	Short: "Import data into PUDL data lake",
	Long: `Import data from files into the PUDL data lake with automatic format detection
and schema assignment.

This command imports data from various formats (JSON, YAML, CSV, NDJSON) and stores it
in the PUDL data lake with full metadata tracking. Raw and metadata files use
content-addressed names.

The --path flag supports both single files and wildcard patterns for batch imports:
- Single file: --path data.json
- Wildcard patterns: --path *.json, --path data/*.yaml, --path logs/2024-*.json

Data Storage:
- In a repository workspace: .pudl/data/{raw,metadata,sqlite}/
- Outside one: ~/.pudl/data/{raw,metadata,sqlite}/
- The repository and global catalogs are separate; imports never cross them

Schema Assignment:
- Manual schema specification with --schema flag (chained validation)
- Automatic schema inference from CUE schemas in the schema repository
- Chained validation: policy → base → generic → catchall
- Never rejects data - always finds appropriate schema
- Envelope wire format: a JSON file shaped like
    {"schema": {"module": "mu/aws", "version": "v1"},
     "definitions": [...],   // optional inline CUE
     "data": <payload>}
  is auto-detected on import. The declared CUE schema ref is recorded
  in the item_schemas table; inline definitions are written to pudl's
  schema cache for future imports. Raw JSON without the envelope shape
  is imported untouched.
- Multiple schemas per item: items can satisfy more than one schema
  (declared, inferred, unresolved) — see 'pudl reclassify --help'.

Compressed Input:
- .gz and .zst files (or gzip/zstd magic bytes) are decompressed before import;
  the decompressed bytes are what is hashed, stored, and parsed

Reading from stdin:
- Pipe data with no --path, or use --path - ; set --format when content
  detection is ambiguous

Example usage:
    # Single file import
    pudl import --path data.json
    pudl import --path aws-instances.json --schema aws.compliant-ec2
    pudl import --path k8s-pods.yaml --schema k8s.pod --origin k8s-get-pods

    # CUE schema reference (e.g. emitted by a mu plugin)
    pudl import --path out.json --schema mu/aws@v1#EC2Instance

    # Wildcard batch import
    pudl import --path *.json
    pudl import --path data/*.yaml
    pudl import --path logs/2024-01-*.json

    # From stdin
    cat data.json | pudl import`,
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
	inference.WarnLoadErrors(os.Stderr, effectiveSchemaPaths(nil)...)

	// Check if reading from stdin
	if filePath == "-" || (filePath == "" && importer.IsStdinAvailable()) {
		return importFromStdin(cmd)
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

	session, err := newImportSession()
	if err != nil {
		return err
	}
	defer session.Close()

	opts := session.options(absPath, workspaceImportOrigin())

	// Perform the import with friendly IDs
	result, err := importOneWithEnvelope(session.imp, opts)
	if err != nil {
		// Print detailed error for debugging
		if os.Getenv("PUDL_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: Import error: %+v\n", err)
		}
		return errors.WrapError(errors.ErrCodeParsingFailed, "Failed to import file", err)
	}

	// Display results
	displayImportResults(result)
	return nil
}

func init() {
	rootCmd.AddCommand(importCmd)

	// Add flags
	importCmd.Flags().StringP("path", "p", "", "Path to file or wildcard pattern to import (use '-' for stdin)")
	importCmd.Flags().StringVar(&importOrigin, "origin", "", "Override origin detection (optional)")
	importCmd.Flags().StringVar(&importSchema, "schema", "", "Specify schema for validation (e.g., aws.compliant-ec2)")
	importCmd.Flags().StringVar(&importFormat, "format", "", "Specify format for stdin data (json, yaml, csv, ndjson)")

	// Retired streaming-parser tuning. Accepted so existing scripts keep
	// working; they have no effect.
	importCmd.Flags().IntVar(&streamingMemoryMB, "streaming-memory", 100, "Deprecated: no effect")
	importCmd.Flags().Float64Var(&streamingChunkMB, "streaming-chunk-size", 0.016, "Deprecated: no effect")
	_ = importCmd.Flags().MarkDeprecated("streaming-memory", "imports no longer use a chunking parser; the flag has no effect")
	_ = importCmd.Flags().MarkDeprecated("streaming-chunk-size", "imports no longer use a chunking parser; the flag has no effect")

	// Register completion functions
	importCmd.RegisterFlagCompletionFunc("schema", completeSchemaNames)
	importCmd.RegisterFlagCompletionFunc("origin", completeOrigins)
}

// displayImportResults shows the results of data import with chained validation info
func displayImportResults(result *importer.ImportResult) {
	// Check if import was skipped due to duplicate
	if result.Skipped {
		fmt.Printf("⏭️  Skipped: %s\n", result.SourcePath)
		fmt.Printf("   Reason: %s\n", result.SkipReason)
		fmt.Printf("   Existing ID: %s\n", result.ID)
		fmt.Printf("   Stored at: %s\n", result.StoredPath)
		return
	}

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

		// Show why the intended schema was not satisfied
		if vr.HasErrors() {
			issues := vr.GetErrorsForSchema(vr.IntendedSchema)
			fmt.Printf("   ❌ Validation Issues: %d\n", len(issues))
			for i, issue := range issues {
				if i == 5 {
					fmt.Printf("      … and %d more\n", len(issues)-i)
					break
				}
				fmt.Printf("      - %s: %s\n", issue.Path, issue.Message)
			}
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

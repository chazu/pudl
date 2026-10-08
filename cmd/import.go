package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/inference"
)

var (
	importExplain     bool
	importSchema      string
	importOrigin      string
	importFormat      string
	importRecursive   bool
	importSet         []string
	importDryRun      bool
	streamingMemoryMB int
	streamingChunkMB  float64
)

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:   "import [paths...] [--path <file|dir|pattern>]",
	Args:  cobra.ArbitraryArgs,
	Short: "Import data into PUDL data lake",
	Long: `Import data from files into the PUDL data lake with automatic format detection
and schema assignment.

This command imports data from various formats (JSON, YAML, CSV, NDJSON) and stores it
in the PUDL data lake with full metadata tracking. Raw and metadata files use
content-addressed names.

Paths may be given as arguments, with --path, or both. Each accepts a single
file, a wildcard pattern, or a directory (an unquoted pattern the shell has
already expanded arrives as several arguments and is imported in full):
- Single file: --path data.json
- Wildcard patterns: --path *.json, --path data/*.yaml, --path logs/2024-*.json
- Directory: --path exports/ imports its .json/.ndjson/.jsonl/.yaml/.yml/.csv
  files (compressed .gz/.zst variants included); add --recursive to descend
  into subdirectories. Hidden files and directories are skipped.

With --json, the command prints one JSON array with an entry per file:
the import result fields plus "status" (imported, skipped, or failed) and,
for a failure, "error". Progress lines go to stderr.

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

    # Several files, or a shell-expanded glob
    pudl import fw-prod-a.json fw-prod-b.json
    pudl import exports/*.json

    # Wildcard batch import
    pudl import --path '*.json'
    pudl import --path data/*.yaml
    pudl import --path logs/2024-01-*.json

    # Check classification, identity and redaction first; nothing is written
    pudl import fw-prod-a.json --schema 'pudl/gcp.#Firewall' --dry-run

    # Add a field gcloud omits, on every record
    pudl import fw-prod-a.json --schema 'pudl/gcp.#Firewall' --set project=prod-a

    # From stdin
    cat data.json | pudl import`,
	RunE: pudlRunE(runImportCommand),
}

// runImportCommand contains the actual import logic with structured error handling
func runImportCommand(cmd *cobra.Command, args []string) error {
	// Get the file path from --path flag
	filePath, err := cmd.Flags().GetString("path")
	if err != nil {
		return errors.WrapError(errors.ErrCodeInvalidInput, "Error getting path flag", err)
	}
	inference.WarnLoadErrors(errw(), effectiveSchemaPaths(nil)...)

	// Named paths — positional args plus --path — win over stdin: an agent
	// with stdin attached still imports the files it named.
	var patterns []string
	for _, arg := range args {
		if arg != "" {
			patterns = append(patterns, arg)
		}
	}
	if filePath != "" && filePath != "-" {
		patterns = append(patterns, filePath)
	}

	// Check if reading from stdin
	if len(patterns) == 0 && (filePath == "-" || importer.IsStdinAvailable()) {
		return importFromStdin(cmd)
	}
	if filePath == "-" {
		return errors.NewInputError("--path - (stdin) cannot be combined with file arguments")
	}

	if len(patterns) == 0 {
		return errors.NewMissingRequiredError("path")
	}

	// Resolve file paths (handles single files, wildcard patterns and directories)
	filePaths, err := resolveAllFilePaths(patterns, importRecursive)
	if err != nil {
		return err
	}

	if len(filePaths) == 0 {
		return errors.NewFileNotFoundError(strings.Join(patterns, ", ") + " (no importable files matched)")
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
	session.ctx = cmd.Context()
	session.limits = importIngestLimits()

	opts := session.options(absPath, workspaceImportOrigin())

	// Perform the import with friendly IDs
	result, err := importOneWithEnvelope(session.imp, opts)
	if jsonOutput {
		if writeErr := writeImportJSON([]importOutcome{newImportOutcome(absPath, result, err)}); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		// Print detailed error for debugging
		if os.Getenv("PUDL_DEBUG") != "" {
			fmt.Fprintf(errw(), "DEBUG: Import error: %+v\n", err)
		}
		return errors.WrapError(errors.ErrCodeParsingFailed, "Failed to import file", err)
	}

	if !jsonOutput {
		displayImportResults(result)
	}
	session.finish()
	return nil
}

func init() {
	rootCmd.AddCommand(importCmd)

	// Add flags
	importCmd.Flags().StringP("path", "p", "", "Path to file or wildcard pattern to import (use '-' for stdin)")
	importCmd.Flags().StringVar(&importOrigin, "origin", "", "Override origin detection (optional)")
	importCmd.Flags().StringVar(&importSchema, "schema", "", "Specify schema for validation (e.g., aws.compliant-ec2)")
	importCmd.Flags().StringVar(&importFormat, "format", "", "Specify format for stdin data (json, yaml, csv, ndjson)")
	importCmd.Flags().BoolVar(&importRecursive, "recursive", false, "When --path is a directory, also import supported files in its subdirectories")

	importCmd.Flags().StringArrayVar(&importSet, "set", nil, "Set a field on every record: path=value (repeatable; overwrites; JSON values keep their type, '\"…\"' forces a string)")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Show how records would be classified, identified, redacted and projected; write nothing")
	importCmd.Flags().BoolVar(&importExplain, "explain", false, "Explain original schema candidates, fallback, and search-path shadowing")

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
	if importExplain {
		displayImportExplanation(result)
	}
	if result.DryRun {
		displayImportPreview(result)
		return
	}
	// Check if import was skipped due to duplicate
	if result.Skipped {
		fmt.Fprintf(outw(), "⏭️  Skipped: %s\n", result.SourcePath)
		fmt.Fprintf(outw(), "   Reason: %s\n", result.SkipReason)
		fmt.Fprintf(outw(), "   Existing ID: %s\n", result.ID)
		fmt.Fprintf(outw(), "   Stored at: %s\n", result.StoredPath)
		printImportNotes(result)
		return
	}

	fmt.Fprintf(outw(), "✅ Successfully imported data!\n")
	fmt.Fprintf(outw(), "   Source: %s\n", result.SourcePath)
	fmt.Fprintf(outw(), "   Stored as: %s\n", result.StoredPath)
	fmt.Fprintf(outw(), "   Format: %s\n", result.DetectedFormat)
	fmt.Fprintf(outw(), "   Origin: %s\n", result.DetectedOrigin)

	// Show validation results if available
	if result.ValidationResult != nil {
		vr := result.ValidationResult
		fmt.Fprintf(outw(), "   %s\n", vr.GetSummary())

		if vr.IntendedSchema != "" && vr.IntendedSchema != vr.AssignedSchema {
			fmt.Fprintf(outw(), "   🎯 Intended Schema: %s\n", vr.IntendedSchema)
		}
		fmt.Fprintf(outw(), "   📋 Assigned Schema: %s\n", vr.AssignedSchema)

		// Show why the intended schema was not satisfied
		if vr.HasErrors() {
			issues := vr.GetErrorsForSchema(vr.IntendedSchema)
			fmt.Fprintf(outw(), "   ❌ Validation Issues: %d\n", len(issues))
			for i, issue := range issues {
				if i == 5 {
					fmt.Fprintf(outw(), "      … and %d more\n", len(issues)-i)
					break
				}
				fmt.Fprintf(outw(), "      - %s: %s\n", issue.Path, issue.Message)
			}
		}
	} else {
		// Display auto-assigned schema
		fmt.Fprintf(outw(), "   Schema: %s\n", result.AssignedSchema)
		if result.SchemaConfidence < 0.8 {
			fmt.Fprintf(outw(), "   ⚠️  Low schema confidence (%.2f) - data assigned to catchall\n", result.SchemaConfidence)
		}
	}

	fmt.Fprintf(outw(), "   Records: %d\n", result.RecordCount)
	fmt.Fprintf(outw(), "   Size: %d bytes\n", result.SizeBytes)
	printImportNotes(result)
}

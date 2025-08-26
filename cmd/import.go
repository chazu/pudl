package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/importer"
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
- Automatic schema detection using Zygomys rule engine
- Falls back to unknown/catchall schema for unclassified data
- Manual schema override with --schema flag (future)

Example usage:
    pudl import --path data.json
    pudl import --path aws-instances.json --origin aws-ec2-describe-instances
    pudl import --path k8s-pods.yaml`,
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

		// Create importer
		imp := importer.New(cfg.DataPath, cfg.SchemaPath)

		// Set up import options
		opts := importer.ImportOptions{
			SourcePath: absPath,
			Origin:     importOrigin, // Will be auto-detected if empty
		}

		// Perform the import
		result, err := imp.ImportFile(opts)
		if err != nil {
			log.Fatalf("Failed to import file: %v", err)
		}

		// Display results
		fmt.Printf("✅ Successfully imported data!\n")
		fmt.Printf("   Source: %s\n", result.SourcePath)
		fmt.Printf("   Stored as: %s\n", result.StoredPath)
		fmt.Printf("   Format: %s\n", result.DetectedFormat)
		fmt.Printf("   Origin: %s\n", result.DetectedOrigin)
		fmt.Printf("   Schema: %s\n", result.AssignedSchema)
		fmt.Printf("   Records: %d\n", result.RecordCount)
		fmt.Printf("   Size: %d bytes\n", result.SizeBytes)
		
		if result.SchemaConfidence < 0.8 {
			fmt.Printf("   ⚠️  Low schema confidence (%.2f) - data assigned to catchall\n", result.SchemaConfidence)
		}
	},
}

func init() {
	rootCmd.AddCommand(importCmd)

	// Add flags
	importCmd.Flags().StringP("path", "p", "", "Path to file to import (required)")
	importCmd.Flags().StringVar(&importOrigin, "origin", "", "Override origin detection (optional)")
	
	// Mark path as required
	importCmd.MarkFlagRequired("path")
}

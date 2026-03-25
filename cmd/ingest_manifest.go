package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/mubridge"
)

var (
	manifestPath   string
	manifestOrigin string
)

var ingestManifestCmd = &cobra.Command{
	Use:   "ingest-manifest",
	Short: "Ingest mu build manifest into the catalog",
	Long: `Read a mu build manifest (JSON) and store action results in the catalog.
This records what mu did during convergence, enabling status tracking.

Examples:
    mu build --emit-manifest | pudl ingest-manifest
    pudl ingest-manifest --path manifest.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine input source
		var reader *os.File
		if manifestPath != "" {
			f, err := os.Open(manifestPath)
			if err != nil {
				return fmt.Errorf("failed to open manifest file: %w", err)
			}
			defer f.Close()
			reader = f
		} else {
			reader = os.Stdin
		}

		// Open the catalog database
		pudlDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(pudlDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog database: %w", err)
		}
		defer db.Close()

		// Ingest the manifest
		result, err := mubridge.IngestManifest(db, reader, manifestOrigin, pudlDir)
		if err != nil {
			return fmt.Errorf("failed to ingest manifest: %w", err)
		}

		// Print summary
		if result.Skipped {
			fmt.Printf("Skipped duplicate manifest (run_id: %s)\n", result.RunID)
		} else {
			fmt.Printf("Ingested manifest (run_id: %s): %d actions (%d cached, %d failed)\n",
				result.RunID, result.Total, result.Cached, result.Failed)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(ingestManifestCmd)

	ingestManifestCmd.Flags().StringVar(&manifestPath, "path", "", "Path to manifest JSON file (reads stdin if omitted)")
	ingestManifestCmd.Flags().StringVar(&manifestOrigin, "origin", "mu-build", "Origin label for catalog entries")
}

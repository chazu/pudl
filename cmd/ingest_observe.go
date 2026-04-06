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
	ingestObservePath   string
	ingestObserveOrigin string
)

var ingestObserveCmd = &cobra.Command{
	Use:   "ingest-observe",
	Short: "Ingest mu observe results into the catalog",
	Long: `Read mu observe --json output and store observed state in the catalog.
The observed state becomes the "live" side for subsequent drift checks.

Input format: JSON array of ObserveResult objects from mu observe --json.
Each target's current.records are stored as individual observe entries,
routed to the correct schema via the _schema field.

Examples:
    mu observe --json //home/odroid | pudl ingest-observe
    pudl ingest-observe --path observe-results.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine input source
		var reader *os.File
		if ingestObservePath != "" {
			f, err := os.Open(ingestObservePath)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", ingestObservePath, err)
			}
			defer f.Close()
			reader = f
		} else {
			reader = os.Stdin
		}

		// Open catalog database
		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		// Load config for data path
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Ingest observe results
		count, err := mubridge.IngestObserveResults(db, reader, ingestObserveOrigin, cfg.DataPath)
		if err != nil {
			return fmt.Errorf("ingest failed: %w", err)
		}

		fmt.Printf("Ingested %d observe results\n", count)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ingestObserveCmd)

	ingestObserveCmd.Flags().StringVar(&ingestObservePath, "path", "", "Read from file instead of stdin")
	ingestObserveCmd.Flags().StringVar(&ingestObserveOrigin, "origin", "mu-observe", "Override origin")
}

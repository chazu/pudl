package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/inference"
)

// verifyCmd represents the verify command
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify schema inference is a fixed point for all catalog entries",
	Long: `Re-run schema inference on all catalog entries and confirm every entry
still resolves to the same schema it was originally assigned.

This is a correctness invariant: if inference is deterministic, re-running it
on stored data should always produce the same schema assignment. Any mismatch
indicates drift between the stored schema and the current inference rules.

Examples:
    # Verify all catalog entries
    pudl verify`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVerifyCommand()
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

func runVerifyCommand() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Derive config dir from data path
	configDir := filepath.Dir(cfg.DataPath)

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return fmt.Errorf("failed to initialize catalog database: %w", err)
	}
	defer catalogDB.Close()

	// Create schema inferrer
	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return fmt.Errorf("failed to initialize schema inferrer: %w", err)
	}

	// Query all catalog entries
	queryResult, err := catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{
		Limit:  0,
		SortBy: "timestamp",
	})
	if err != nil {
		return fmt.Errorf("failed to query catalog entries: %w", err)
	}

	entries := queryResult.Entries
	if len(entries) == 0 {
		fmt.Println("No catalog entries to verify.")
		return nil
	}

	fmt.Printf("Verifying %d catalog entries...\n", len(entries))

	var okCount, mismatchCount, errCount int

	for _, entry := range entries {
		displayName := filepath.Base(entry.StoredPath)

		// Load data from stored path
		data, err := loadVerifyData(entry.StoredPath)
		if err != nil {
			fmt.Printf("  %s: ERROR (failed to load data: %v)\n", displayName, err)
			errCount++
			continue
		}

		// Determine collection type for inference hints
		collectionType := ""
		if entry.CollectionType != nil {
			collectionType = *entry.CollectionType
		}

		// Re-run inference
		result, err := inferrer.Infer(data, inference.InferenceHints{
			Origin:         entry.Origin,
			Format:         entry.Format,
			CollectionType: collectionType,
		})
		if err != nil {
			fmt.Printf("  %s: ERROR (inference failed: %v)\n", displayName, err)
			errCount++
			continue
		}

		if result.Schema == entry.Schema {
			fmt.Printf("  %s: OK (%s)\n", displayName, entry.Schema)
			okCount++
		} else {
			fmt.Printf("  %s: MISMATCH (stored: %s, inferred: %s)\n",
				displayName, entry.Schema, result.Schema)
			mismatchCount++
		}
	}

	fmt.Println()
	fmt.Printf("Result: %d OK, %d mismatch", okCount, mismatchCount)
	if errCount > 0 {
		fmt.Printf(", %d errors", errCount)
	}
	fmt.Println()

	if mismatchCount > 0 {
		return fmt.Errorf("fixed-point verification failed: %d mismatches found", mismatchCount)
	}

	return nil
}

// loadVerifyData loads data from a stored file path for verification.
func loadVerifyData(storedPath string) (interface{}, error) {
	data, err := os.ReadFile(storedPath)
	if err != nil {
		return nil, err
	}

	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		// If not valid JSON, return as string
		return string(data), nil
	}

	return jsonData, nil
}

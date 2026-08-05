package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// verifyCmd represents the verify command
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify catalog schemas and inferred assignments",
	Long: `Validate every catalog entry against its assigned schema.

For ordinary imported records, also re-run schema inference and confirm it is a
fixed point. Explicitly typed records (including plugin and bridge artifacts)
are checked against their assigned schema; their payload is not reclassified by
heuristic inference.

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
	cfg, err := loadEffectiveConfig()
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
	inferrer, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return fmt.Errorf("failed to initialize schema inferrer: %w", err)
	}
	validationService, err := validator.NewValidationService(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return fmt.Errorf("failed to initialize validation service: %w", err)
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

		validation := validationService.ValidateDataAgainstSchema(data, entry.Schema)
		if !validation.Valid {
			fmt.Printf("  %s: INVALID (%s): %s\n", displayName, entry.Schema, validation.ErrorMessage)
			for _, validationErr := range validation.Errors {
				fmt.Printf("      - %s\n", validationErr)
			}
			errCount++
			continue
		}

		if !shouldVerifyInference(data, entry) {
			fmt.Printf("  %s: OK (%s; explicit)\n", displayName, entry.Schema)
			okCount++
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

		if schemaname.IsEquivalent(result.Schema, entry.Schema) {
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

	if mismatchCount > 0 || errCount > 0 {
		return fmt.Errorf("catalog verification failed: %d mismatches, %d errors", mismatchCount, errCount)
	}

	return nil
}

// shouldVerifyInference reports whether the stored assignment should be
// re-derived. Explicitly typed and bridge-owned records already have an
// authoritative producer assignment; ordinary imported records are the ones
// for which fixed-point inference is meaningful.
func shouldVerifyInference(data interface{}, entry database.CatalogEntry) bool {
	if entry.EntryType != nil || entry.CollectionType != nil {
		return false
	}

	if record, ok := data.(map[string]interface{}); ok {
		if declared, ok := record["_schema"].(string); ok && strings.TrimSpace(declared) != "" {
			return false
		}
	}

	return true
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

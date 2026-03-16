package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/errors"
	"pudl/internal/idgen"
	"pudl/internal/validator"
)

var (
	validateAll   bool
	validateEntry string
)

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate catalog data against assigned schemas",
	Long: `Validate catalog entries against their assigned CUE schemas.

This command checks whether imported data conforms to the schema it has been
assigned. It can validate individual entries or all entries in the catalog.

Examples:
    pudl validate --entry babod-fakak      # Validate a specific entry by proquint
    pudl validate --all                     # Validate all catalog entries
    pudl validate --all --verbose           # Validate all with detailed output`,
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)

		if err := runValidateCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)

	validateCmd.Flags().BoolVar(&validateAll, "all", false, "Validate all catalog entries")
	validateCmd.Flags().StringVar(&validateEntry, "entry", "", "Validate a specific entry by proquint ID")

	// At least one of --all or --entry must be specified
	validateCmd.MarkFlagsOneRequired("all", "entry")
	validateCmd.MarkFlagsMutuallyExclusive("all", "entry")

	// Register completion functions
	validateCmd.RegisterFlagCompletionFunc("entry", completeProquintIDs)
}

func runValidateCommand(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Derive config dir from data path (data path is typically ~/.pudl/data, so config dir is ~/.pudl)
	configDir := filepath.Dir(cfg.DataPath)

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return errors.NewSystemError("Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	// Create validation service
	validationService, err := validator.NewValidationService(cfg.SchemaPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize validation service", err)
	}

	if validateAll {
		return validateAllEntries(catalogDB, validationService)
	}

	return validateSingleEntry(catalogDB, validationService, validateEntry)
}

// validateSingleEntry validates a single catalog entry by proquint ID
func validateSingleEntry(catalogDB *database.CatalogDB, vs *validator.ValidationService, proquintID string) error {
	// Look up entry by proquint
	entry, err := catalogDB.GetEntryByProquint(proquintID)
	if err != nil {
		// Try direct ID lookup as fallback
		entry, err = catalogDB.GetEntry(proquintID)
		if err != nil {
			return errors.NewInputError(
				fmt.Sprintf("Entry not found: %s", proquintID),
				"Check the entry ID with 'pudl list'",
				"Ensure you're using the correct proquint identifier")
		}
	}

	// Load data from stored path
	data, err := loadDataFromFile(entry.StoredPath)
	if err != nil {
		return errors.NewSystemError(fmt.Sprintf("Failed to load data from %s", entry.StoredPath), err)
	}

	// Validate against assigned schema
	result := vs.ValidateDataAgainstSchema(data, entry.Schema)

	// Display result
	proquint := idgen.HashToProquint(entry.ID)
	fmt.Printf("Validating entry: %s\n", proquint)
	fmt.Printf("Schema: %s\n", entry.Schema)
	fmt.Printf("Data path: %s\n", entry.StoredPath)
	fmt.Println()

	fmt.Print(vs.GetValidationSummary(result))

	if !result.Valid {
		return errors.NewValidationError(
			entry.Schema,
			result.Errors,
			nil)
	}

	return nil
}

// validateAllEntries validates all entries in the catalog
func validateAllEntries(catalogDB *database.CatalogDB, vs *validator.ValidationService) error {
	// Query all entries
	queryResult, err := catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{
		Limit:   0, // No limit
		SortBy:  "import_timestamp",
		Reverse: true,
	})
	if err != nil {
		return errors.NewSystemError("Failed to query catalog entries", err)
	}

	if len(queryResult.Entries) == 0 {
		fmt.Println("No entries found in catalog.")
		return nil
	}

	fmt.Printf("Validating %d entries...\n\n", len(queryResult.Entries))

	var validCount, invalidCount, errorCount int
	var invalidEntries []invalidEntry

	for _, entry := range queryResult.Entries {
		proquint := idgen.HashToProquint(entry.ID)

		// Load data from stored path
		data, err := loadDataFromFile(entry.StoredPath)
		if err != nil {
			fmt.Printf("  %s [%s] - ERROR: Failed to load data\n", proquint, entry.Schema)
			errorCount++
			continue
		}

		// Validate against assigned schema
		result := vs.ValidateDataAgainstSchema(data, entry.Schema)

		if result.Valid {
			fmt.Printf("  %s [%s] - VALID\n", proquint, entry.Schema)
			validCount++
		} else {
			fmt.Printf("  %s [%s] - INVALID\n", proquint, entry.Schema)
			// Show validation errors inline for immediate feedback
			if result.ErrorMessage != "" {
				fmt.Printf("      Reason: %s\n", result.ErrorMessage)
			}
			for _, e := range result.Errors {
				fmt.Printf("      - %s\n", e)
			}
			invalidCount++
			invalidEntries = append(invalidEntries, invalidEntry{
				Proquint: proquint,
				Schema:   entry.Schema,
				Result:   result,
			})
		}
	}

	// Summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("Validation Summary")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Total entries: %d\n", len(queryResult.Entries))
	fmt.Printf("Valid:         %d\n", validCount)
	fmt.Printf("Invalid:       %d\n", invalidCount)
	fmt.Printf("Errors:        %d\n", errorCount)

	// List invalid entry IDs for easy reference
	if len(invalidEntries) > 0 {
		fmt.Println()
		fmt.Println("Invalid entry IDs:")
		for _, inv := range invalidEntries {
			fmt.Printf("  - %s\n", inv.Proquint)
		}
	}

	if invalidCount > 0 || errorCount > 0 {
		return errors.NewInputError(
			fmt.Sprintf("Validation failed: %d invalid, %d errors", invalidCount, errorCount),
			"Run 'pudl schema reinfer --all' to update schema assignments")
	}

	fmt.Println()
	fmt.Println("All entries valid!")
	return nil
}

type invalidEntry struct {
	Proquint string
	Schema   string
	Result   *validator.ServiceValidationResult
}

// loadDataFromFile loads data from a stored file path
func loadDataFromFile(storedPath string) (interface{}, error) {
	data, err := os.ReadFile(storedPath)
	if err != nil {
		return nil, err
	}

	// Parse as JSON (most common format)
	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		// If JSON parsing fails, return as string
		return string(data), nil
	}

	return jsonData, nil
}

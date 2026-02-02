package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/errors"
	"pudl/internal/idgen"
	"pudl/internal/review"
)

var (
	revalidateAll    bool
	revalidateSchema string
	revalidateID     string
	revalidateOrigin string
	revalidateDryRun bool
)

// revalidateCmd represents the revalidate command
var revalidateCmd = &cobra.Command{
	Use:   "revalidate",
	Short: "Batch revalidate catalog entries against schemas",
	Long: `Revalidate catalog entries against their assigned schemas.

This command allows you to revalidate multiple entries in your catalog,
optionally updating their schema assignments if they now validate against
different schemas. Use --dry-run to preview changes without applying them.

Examples:
    pudl revalidate --all                    # Revalidate all entries
    pudl revalidate --schema aws.ec2         # Revalidate entries with specific schema
    pudl revalidate --origin aws --dry-run   # Preview changes for AWS entries
    pudl revalidate --id babod-fakak         # Revalidate specific entry`,
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)

		if err := runRevalidateCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(revalidateCmd)

	revalidateCmd.Flags().BoolVar(&revalidateAll, "all", false, "Revalidate all catalog entries")
	revalidateCmd.Flags().StringVar(&revalidateSchema, "schema", "", "Filter by schema name")
	revalidateCmd.Flags().StringVar(&revalidateID, "id", "", "Revalidate specific entry by proquint")
	revalidateCmd.Flags().StringVar(&revalidateOrigin, "origin", "", "Filter by data origin")
	revalidateCmd.Flags().BoolVar(&revalidateDryRun, "dry-run", false, "Preview changes without applying them")

	// At least one filter must be specified
	revalidateCmd.MarkFlagsOneRequired("all", "schema", "id", "origin")

	// Register completion functions
	revalidateCmd.RegisterFlagCompletionFunc("schema", completeSchemaNames)
	revalidateCmd.RegisterFlagCompletionFunc("origin", completeOrigins)
	revalidateCmd.RegisterFlagCompletionFunc("id", completeProquintIDs)
}

func runRevalidateCommand(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return errors.NewSystemError("Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	// Create validation service
	validationService, err := review.NewValidationService(cfg.SchemaPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize validation service", err)
	}

	// Build filter options
	filters := database.FilterOptions{
		Schema: revalidateSchema,
		Origin: revalidateOrigin,
	}

	// Query entries
	var entries []database.CatalogEntry
	if revalidateID != "" {
		// Single entry lookup
		entry, err := catalogDB.GetEntryByProquint(revalidateID)
		if err != nil {
			entry, err = catalogDB.GetEntry(revalidateID)
			if err != nil {
				return errors.NewInputError(
					fmt.Sprintf("Entry not found: %s", revalidateID),
					"Check the entry ID with 'pudl list'")
			}
		}
		entries = append(entries, *entry)
	} else {
		// Query with filters
		result, err := catalogDB.QueryEntries(filters, database.QueryOptions{})
		if err != nil {
			return errors.NewSystemError("Failed to query catalog entries", err)
		}
		entries = result.Entries
	}

	if len(entries) == 0 {
		fmt.Println("No entries found matching the criteria.")
		return nil
	}

	// Revalidate entries
	return revalidateEntries(catalogDB, validationService, entries, revalidateDryRun)
}

func revalidateEntries(catalogDB *database.CatalogDB, vs *review.ValidationService, entries []database.CatalogEntry, dryRun bool) error {
	fmt.Printf("Revalidating %d entries...\n\n", len(entries))

	var passed, failed, schemaChanged int
	var changes []struct {
		proquint  string
		oldSchema string
		newSchema string
	}

	for i, entry := range entries {
		proquint := idgen.HashToProquint(entry.ID)
		fmt.Printf("[%d/%d] %s [%s]... ", i+1, len(entries), proquint, entry.Schema)

		// Load data from stored path
		data, err := loadDataFromFile(entry.StoredPath)
		if err != nil {
			fmt.Printf("ERROR: Failed to load data\n")
			failed++
			continue
		}

		// Validate against assigned schema
		result := vs.ValidateDataAgainstSchema(data, entry.Schema)

		if result.Valid {
			fmt.Printf("VALID\n")
			passed++
		} else {
			fmt.Printf("INVALID (assigned to %s)\n", result.AssignedSchema)
			if result.AssignedSchema != entry.Schema {
				schemaChanged++
				changes = append(changes, struct {
					proquint  string
					oldSchema string
					newSchema string
				}{proquint, entry.Schema, result.AssignedSchema})

				// Update schema if not dry-run
				if !dryRun {
					entry.Schema = result.AssignedSchema
					if err := catalogDB.UpdateEntry(entry); err != nil {
						fmt.Printf("  Warning: Failed to update schema: %v\n", err)
					}
				}
			}
			failed++
		}
	}

	// Summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("Revalidation Summary")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Total entries:    %d\n", len(entries))
	fmt.Printf("Passed:           %d\n", passed)
	fmt.Printf("Failed:           %d\n", failed)
	fmt.Printf("Schema changes:   %d\n", schemaChanged)

	if dryRun {
		fmt.Println("\n(DRY RUN - No changes applied)")
	}

	if len(changes) > 0 {
		fmt.Println("\nSchema changes:")
		for _, change := range changes {
			fmt.Printf("  %s: %s → %s\n", change.proquint, change.oldSchema, change.newSchema)
		}
	}

	return nil
}

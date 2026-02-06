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
	"pudl/internal/identity"
	"pudl/internal/inference"
)

var identityMigrateDryRun bool

var identityMigrateCmd = &cobra.Command{
	Use:   "identity",
	Short: "Backfill resource identity tracking for existing entries",
	Long: `Compute and populate resource_id and identity_json for catalog entries
that were imported before identity tracking was added.

The auto-migration sets content_hash and version on DB open, but resource_id
requires loading each entry's data file and extracting identity fields from
the schema metadata. This command does that full computation.

Examples:
    pudl migrate identity              # Backfill all entries with NULL resource_id
    pudl migrate identity --dry-run    # Preview what would be updated`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIdentityMigrate()
	},
}

func init() {
	migrateCmd.AddCommand(identityMigrateCmd)
	identityMigrateCmd.Flags().BoolVar(&identityMigrateDryRun, "dry-run", false, "Preview changes without applying")
}

func runIdentityMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to load configuration", err)
	}

	configDir := filepath.Dir(cfg.DataPath)

	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return errors.NewSystemError("Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize schema inferrer", err)
	}

	// Query all entries with NULL resource_id
	result, err := catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 0})
	if err != nil {
		return errors.NewSystemError("Failed to query catalog", err)
	}

	var needsUpdate []database.CatalogEntry
	for _, entry := range result.Entries {
		if entry.ResourceID == nil {
			needsUpdate = append(needsUpdate, entry)
		}
	}

	if len(needsUpdate) == 0 {
		fmt.Println("All entries already have resource_id. Nothing to migrate.")
		return nil
	}

	fmt.Printf("Found %d entries without resource_id\n\n", len(needsUpdate))

	var successCount, failCount int
	for _, entry := range needsUpdate {
		// Load data from stored file
		dataBytes, err := os.ReadFile(entry.StoredPath)
		if err != nil {
			fmt.Printf("  SKIP %s: cannot read data file: %v\n", entry.ID[:16], err)
			failCount++
			continue
		}

		var data interface{}
		if err := json.Unmarshal(dataBytes, &data); err != nil {
			// Not valid JSON — use content hash as identity
			data = nil
		}

		// Get identity fields from schema metadata
		var identityFields []string
		if meta, found := inferrer.GetSchemaMetadata(entry.Schema); found {
			identityFields = meta.IdentityFields
		}

		// Determine content hash
		contentHash := entry.ID
		if entry.ContentHash != nil {
			contentHash = *entry.ContentHash
		}

		// Extract identity values
		var identityValues map[string]interface{}
		if data != nil && len(identityFields) > 0 {
			identityValues, _ = identity.ExtractFieldValues(data, identityFields)
		}

		// Compute resource_id
		resourceID := identity.ComputeResourceID(entry.Schema, identityValues, contentHash)

		// Compute identity_json
		var identityJSON *string
		if identityValues != nil && len(identityValues) > 0 {
			if canonical, err := identity.CanonicalIdentityJSON(identityValues); err == nil {
				identityJSON = &canonical
			}
		}

		if identityMigrateDryRun {
			fmt.Printf("  WOULD UPDATE %s: resource_id=%s\n", entry.ID[:16], resourceID[:16])
			successCount++
			continue
		}

		// Update entry
		entry.ResourceID = &resourceID
		entry.IdentityJSON = identityJSON
		if err := catalogDB.UpdateEntry(entry); err != nil {
			fmt.Printf("  FAIL %s: %v\n", entry.ID[:16], err)
			failCount++
			continue
		}

		fmt.Printf("  OK %s: resource_id=%s\n", entry.ID[:16], resourceID[:16])
		successCount++
	}

	fmt.Printf("\nMigration complete: %d updated, %d failed\n", successCount, failCount)
	if identityMigrateDryRun {
		fmt.Println("(dry run — no changes applied)")
	}

	return nil
}

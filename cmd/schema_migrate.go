package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/errors"
)

// schemaMigrateCmd represents the schema migrate command
var schemaMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate catalog schema names to canonical format",
	Long: `Migrate all schema names in the catalog database to canonical format.

This command normalizes schema names from various formats to the canonical format:
  <package>.<#Definition>  (e.g., aws/ec2.#Instance, pudl/core.#Item)

Input formats that are normalized:
  - pudl.schemas/aws/ec2@v0:#Instance  -> aws/ec2.#Instance
  - pudl.schemas/aws/ec2:#Instance     -> aws/ec2.#Instance
  - aws/ec2:#Instance                  -> aws/ec2.#Instance
  - core.#Item                         -> pudl/core.#Item

This migration is idempotent - running it multiple times is safe.

Examples:
    # Migrate all schema names
    pudl schema migrate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaMigrateCommand()
	},
}

func init() {
	schemaCmd.AddCommand(schemaMigrateCmd)
}

// runSchemaMigrateCommand migrates schema names to canonical format
func runSchemaMigrateCommand() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to load configuration", err)
	}

	// Derive config dir from data path
	configDir := filepath.Dir(cfg.DataPath)

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return errors.NewSystemError("Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	fmt.Printf("🔄 Schema Name Migration\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	// Run migration
	count, err := catalogDB.MigrateSchemaNames()
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to migrate schema names", err)
	}

	if count == 0 {
		fmt.Println("✅ All schema names are already in canonical format.")
	} else {
		fmt.Printf("✅ Migrated %d schema names to canonical format.\n", count)
	}

	return nil
}

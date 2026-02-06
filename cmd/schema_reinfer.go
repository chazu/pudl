package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/errors"
	"pudl/internal/idgen"
	"pudl/internal/identity"
	"pudl/internal/inference"
)

var (
	// Reinfer command flags
	reinferAll    bool
	reinferEntry  string
	reinferSchema string
	reinferOrigin string
	reinferDryRun bool
	reinferForce  bool
)

// schemaReinferCmd represents the schema reinfer command
var schemaReinferCmd = &cobra.Command{
	Use:   "reinfer",
	Short: "Re-run schema inference on existing catalog entries",
	Long: `Re-run schema inference on data that has already been imported.

This command is useful when:
- New schemas have been added and you want existing data to match against them
- Schema cascade priorities have been modified
- You want to batch-update schema assignments without interactive review

The command will re-analyze each matching entry and update its schema assignment
if a better match is found.

Examples:
    # Re-infer all entries currently assigned to unknown schema
    pudl schema reinfer --schema core.#Item

    # Re-infer a specific entry
    pudl schema reinfer --entry babod-fakak

    # Preview what would change for all entries
    pudl schema reinfer --all --dry-run

    # Re-infer all entries from a specific origin
    pudl schema reinfer --origin aws-ec2

    # Force re-inference without confirmation
    pudl schema reinfer --all --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaReinferCommand()
	},
}

func init() {
	schemaCmd.AddCommand(schemaReinferCmd)

	schemaReinferCmd.Flags().BoolVar(&reinferAll, "all", false, "Re-infer schemas for all catalog entries")
	schemaReinferCmd.Flags().StringVar(&reinferEntry, "entry", "", "Re-infer schema for a specific entry by proquint ID")
	schemaReinferCmd.Flags().StringVar(&reinferSchema, "schema", "", "Re-infer only entries currently assigned to this schema")
	schemaReinferCmd.Flags().StringVar(&reinferOrigin, "origin", "", "Re-infer only entries from a specific origin")
	schemaReinferCmd.Flags().BoolVar(&reinferDryRun, "dry-run", false, "Show what would change without applying updates")
	schemaReinferCmd.Flags().BoolVar(&reinferForce, "force", false, "Apply changes without confirmation prompt")

	schemaReinferCmd.RegisterFlagCompletionFunc("entry", completeProquintIDs)
	schemaReinferCmd.RegisterFlagCompletionFunc("schema", completeSchemaNames)
	schemaReinferCmd.RegisterFlagCompletionFunc("origin", completeOrigins)
}

// runSchemaReinferCommand runs the schema reinfer command
func runSchemaReinferCommand() error {
	// Validate that at least one filter is specified
	if !reinferAll && reinferEntry == "" && reinferSchema == "" && reinferOrigin == "" {
		return errors.NewInputError(
			"At least one filter must be specified",
			"Use --all, --entry, --schema, or --origin to select entries to re-infer")
	}

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

	// Create schema inferrer
	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize schema inferrer", err)
	}

	fmt.Printf("🔄 Schema Re-inference\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	// Handle single entry case
	if reinferEntry != "" {
		return reinferSingleEntry(catalogDB, inferrer, reinferEntry)
	}

	// Build filter for batch operation
	filter := database.FilterOptions{
		Schema: reinferSchema,
		Origin: reinferOrigin,
	}

	// Query matching entries
	queryResult, err := catalogDB.QueryEntries(filter, database.QueryOptions{
		Limit:   0, // No limit
		SortBy:  "import_timestamp",
		Reverse: true,
	})
	if err != nil {
		return errors.NewSystemError("Failed to query catalog entries", err)
	}

	if len(queryResult.Entries) == 0 {
		fmt.Println("No entries found matching the specified criteria.")
		return nil
	}

	fmt.Printf("Found %d entries to process\n\n", len(queryResult.Entries))

	// Analyze what would change
	changes := analyzeReinferChanges(queryResult.Entries, catalogDB, inferrer, cfg.DataPath)

	// Show summary
	printReinferSummary(changes)

	if reinferDryRun {
		fmt.Println("\n🔍 Dry run - no changes applied")
		return nil
	}

	if len(changes.updated) == 0 {
		fmt.Println("\n✅ No schema changes needed")
		return nil
	}

	// Confirm before applying
	if !reinferForce {
		fmt.Printf("\nApply %d schema changes? [y/N]: ", len(changes.updated))
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Apply changes
	return applyReinferChanges(catalogDB, inferrer, changes)
}

// reinferSingleEntry re-infers schema for a single entry
func reinferSingleEntry(catalogDB *database.CatalogDB, inferrer *inference.SchemaInferrer, proquintID string) error {
	// Look up entry by proquint
	entry, err := catalogDB.GetEntryByProquint(proquintID)
	if err != nil {
		entry, err = catalogDB.GetEntry(proquintID)
		if err != nil {
			return errors.NewInputError(
				fmt.Sprintf("Entry not found: %s", proquintID),
				"Check the entry ID with 'pudl list'")
		}
	}

	// Load data from stored path
	data, err := loadReinferData(entry.StoredPath)
	if err != nil {
		return errors.NewSystemError(fmt.Sprintf("Failed to load data from %s", entry.StoredPath), err)
	}

	// Determine collection type for inference hints
	collectionType := ""
	if entry.CollectionType != nil {
		collectionType = *entry.CollectionType
	}

	// Run inference
	result, err := inferrer.Infer(data, inference.InferenceHints{
		Origin:         entry.Origin,
		Format:         entry.Format,
		CollectionType: collectionType,
	})
	if err != nil {
		return errors.NewSystemError("Schema inference failed", err)
	}

	proquint := idgen.HashToProquint(entry.ID)
	fmt.Printf("Entry: %s\n", proquint)
	fmt.Printf("Current schema: %s\n", entry.Schema)
	fmt.Printf("Inferred schema: %s (confidence: %.2f)\n", result.Schema, result.Confidence)

	if result.Schema == entry.Schema {
		fmt.Println("\n✅ Schema unchanged")
		return nil
	}

	if reinferDryRun {
		fmt.Println("\n🔍 Dry run - no changes applied")
		return nil
	}

	if !reinferForce {
		fmt.Printf("\nUpdate schema to %s? [y/N]: ", result.Schema)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Update the entry directly, recomputing identity with new schema
	updatedEntry, err := catalogDB.GetEntry(entry.ID)
	if err != nil {
		return errors.NewSystemError("Failed to get catalog entry for update", err)
	}
	updatedEntry.Schema = result.Schema
	updatedEntry.Confidence = result.Confidence

	// Recompute identity with the new schema
	recomputeEntryIdentity(updatedEntry, data, inferrer)

	if err := catalogDB.UpdateEntry(*updatedEntry); err != nil {
		return errors.NewSystemError("Failed to update catalog entry", err)
	}

	fmt.Printf("\n✅ Updated schema: %s → %s\n", entry.Schema, result.Schema)
	return nil
}

// reinferChanges holds the analysis of what would change
type reinferChanges struct {
	updated   []reinferChange
	unchanged []string
	errors    []string
}

type reinferChange struct {
	entryID    string
	proquint   string
	oldSchema  string
	newSchema  string
	confidence float64
	data       interface{} // loaded data for identity recomputation
}

// analyzeReinferChanges analyzes what schema changes would occur
func analyzeReinferChanges(entries []database.CatalogEntry, catalogDB *database.CatalogDB, inferrer *inference.SchemaInferrer, dataPath string) *reinferChanges {
	changes := &reinferChanges{
		updated:   []reinferChange{},
		unchanged: []string{},
		errors:    []string{},
	}

	for _, entry := range entries {
		proquint := idgen.HashToProquint(entry.ID)

		// Load data
		data, err := loadReinferData(entry.StoredPath)
		if err != nil {
			changes.errors = append(changes.errors, fmt.Sprintf("%s: failed to load data", proquint))
			continue
		}

		// Determine collection type for inference hints
		collectionType := ""
		if entry.CollectionType != nil {
			collectionType = *entry.CollectionType
		}

		// Run inference
		result, err := inferrer.Infer(data, inference.InferenceHints{
			Origin:         entry.Origin,
			Format:         entry.Format,
			CollectionType: collectionType,
		})
		if err != nil {
			changes.errors = append(changes.errors, fmt.Sprintf("%s: inference failed", proquint))
			continue
		}

		if result.Schema != entry.Schema {
			changes.updated = append(changes.updated, reinferChange{
				entryID:    entry.ID,
				proquint:   proquint,
				oldSchema:  entry.Schema,
				newSchema:  result.Schema,
				confidence: result.Confidence,
				data:       data,
			})
		} else {
			changes.unchanged = append(changes.unchanged, proquint)
		}
	}

	return changes
}

// printReinferSummary prints a summary of the reinfer analysis
func printReinferSummary(changes *reinferChanges) {
	fmt.Printf("📊 Analysis Summary:\n")
	fmt.Printf("   Would update: %d entries\n", len(changes.updated))
	fmt.Printf("   Unchanged:    %d entries\n", len(changes.unchanged))
	fmt.Printf("   Errors:       %d entries\n", len(changes.errors))

	if len(changes.updated) > 0 {
		fmt.Printf("\n📝 Schema changes:\n")
		for _, change := range changes.updated {
			fmt.Printf("   %s: %s → %s (%.2f)\n",
				change.proquint, change.oldSchema, change.newSchema, change.confidence)
		}
	}

	if len(changes.errors) > 0 {
		fmt.Printf("\n⚠️  Errors:\n")
		for _, errMsg := range changes.errors {
			fmt.Printf("   %s\n", errMsg)
		}
	}
}

// applyReinferChanges applies the analyzed schema changes
func applyReinferChanges(catalogDB *database.CatalogDB, inferrer *inference.SchemaInferrer, changes *reinferChanges) error {
	var successCount, failCount int
	for _, change := range changes.updated {
		entry, err := catalogDB.GetEntry(change.entryID)
		if err != nil {
			fmt.Printf("   ❌ %s: failed to update\n", change.proquint)
			failCount++
			continue
		}
		entry.Schema = change.newSchema
		entry.Confidence = change.confidence

		// Recompute identity with the new schema
		recomputeEntryIdentity(entry, change.data, inferrer)

		if err := catalogDB.UpdateEntry(*entry); err != nil {
			fmt.Printf("   ❌ %s: failed to update\n", change.proquint)
			failCount++
		} else {
			fmt.Printf("   ✅ %s: %s → %s\n", change.proquint, change.oldSchema, change.newSchema)
			successCount++
		}
	}

	fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("✅ Updated: %d entries\n", successCount)
	if failCount > 0 {
		fmt.Printf("❌ Failed:  %d entries\n", failCount)
	}

	return nil
}

// recomputeEntryIdentity recomputes resource_id and identity_json for an entry
// using its current schema and loaded data. Version number stays the same
// since reinfer is reclassification, not a new observation.
func recomputeEntryIdentity(entry *database.CatalogEntry, data interface{}, inferrer *inference.SchemaInferrer) {
	// Get identity fields from the new schema's metadata
	var identityFields []string
	if meta, found := inferrer.GetSchemaMetadata(entry.Schema); found {
		identityFields = meta.IdentityFields
	}

	// Determine content hash — use existing or fall back to entry ID
	contentHash := entry.ID
	if entry.ContentHash != nil {
		contentHash = *entry.ContentHash
	}

	// Extract identity values
	identityValues, err := identity.ExtractFieldValues(data, identityFields)
	if err != nil {
		identityValues = nil
	}

	// Compute new resource_id
	resourceID := identity.ComputeResourceID(entry.Schema, identityValues, contentHash)
	entry.ResourceID = &resourceID

	// Compute new identity_json
	if identityValues != nil && len(identityValues) > 0 {
		if canonical, err := identity.CanonicalIdentityJSON(identityValues); err == nil {
			entry.IdentityJSON = &canonical
		}
	} else {
		entry.IdentityJSON = nil
	}
}

// loadReinferData loads data from a stored file path for re-inference
func loadReinferData(storedPath string) (interface{}, error) {
	data, err := os.ReadFile(storedPath)
	if err != nil {
		return nil, err
	}

	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return string(data), nil
	}

	return jsonData, nil
}

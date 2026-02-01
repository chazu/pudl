package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/idgen"
	"pudl/internal/lister"
	"pudl/internal/ui"
)

var (
	listSchema        string
	listOrigin        string
	listFormat        string
	listVerbose       bool
	listLimit         int
	listSortBy        string
	listReverse       bool
	listCollectionID  string
	listCollectionType string
	listItemID        string
	listCollectionsOnly bool
	listItemsOnly     bool
	listFancy         bool
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List imported data in the PUDL data lake",
	Long: `List and query imported data in the PUDL data lake with filtering and sorting options.

This command displays information about all imported data, including metadata such as
schema assignments, import timestamps, file sizes, and record counts. You can filter
the results by various criteria and sort them in different ways.

Filtering Options:
- --schema: Filter by CUE schema (e.g., aws.#EC2Instance, k8s.#Pod)
- --origin: Filter by data origin (e.g., aws-ec2, k8s-pods)
- --format: Filter by file format (json, yaml, csv, ndjson)
- --collection-id: Filter by collection ID (show items from specific collection)
- --collections-only: Show only collection entries (not individual items)
- --items-only: Show only individual items (not collections)
- --item-id: Filter by specific item ID

Display Options:
- --verbose: Show detailed information including file paths
- --limit: Limit number of results (default: 50)
- --sort-by: Sort by field (timestamp, size, records, schema, origin)
- --reverse: Reverse sort order
- --fancy: Use interactive bubbletea interface with filtering (press / to filter, enter to show details with raw data)

Examples:
    pudl list                                    # List all imported data
    pudl list --schema aws.#EC2Instance          # List only EC2 instances
    pudl list --origin k8s-pods                  # List Kubernetes pod data
    pudl list --format ndjson --verbose         # List NDJSON collections with details
    pudl list --collections-only                # Show only collections
    pudl list --items-only                      # Show only individual items
    pudl list --collection-id my-collection     # Show items from specific collection
    pudl list --sort-by size --reverse          # List by size, largest first
    pudl list --limit 10                        # Show only first 10 entries
    pudl list --fancy                           # Interactive list with filtering and detailed view`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the list command and handle any errors
		if err := runListCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runListCommand contains the actual list logic with structured error handling
func runListCommand(cmd *cobra.Command, args []string) error {
	// Load configuration to get data directory
	cfg, err := config.Load()
	if err != nil {
		return err // Already a PUDLError from config.Load()
	}

	// Create lister
	l, err := lister.New(cfg.DataPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize lister", err)
	}
	defer l.Close()

	// Set up filter options
	filters := lister.FilterOptions{
		Schema:         listSchema,
		Origin:         listOrigin,
		Format:         listFormat,
		CollectionID:   listCollectionID,
		CollectionType: determineCollectionType(),
		ItemID:         listItemID,
	}

	// Set up display options
	displayOpts := lister.DisplayOptions{
		Verbose: listVerbose,
		Limit:   listLimit,
		SortBy:  listSortBy,
		Reverse: listReverse,
	}

	// List data
	results, err := l.ListData(filters, displayOpts)
	if err != nil {
		return err // Already a PUDLError from lister
	}

	// Display results
	if len(results.Entries) == 0 {
		fmt.Println("No data found matching the specified criteria.")
		return nil
	}

	// Use fancy bubbletea UI if requested
	if listFancy {
		return ui.RunInteractiveList(results.Entries, listVerbose)
	}

	// Traditional text output
	// Display summary
	fmt.Printf("Found %d entries", len(results.Entries))
	if results.TotalEntries > len(results.Entries) {
		fmt.Printf(" (showing %d of %d total)", len(results.Entries), results.TotalEntries)
	}
	fmt.Println()

	// Display filters if any are active
	activeFilters := []string{}
	if listSchema != "" {
		activeFilters = append(activeFilters, fmt.Sprintf("schema=%s", listSchema))
	}
	if listOrigin != "" {
		activeFilters = append(activeFilters, fmt.Sprintf("origin=%s", listOrigin))
	}
	if listFormat != "" {
		activeFilters = append(activeFilters, fmt.Sprintf("format=%s", listFormat))
	}
	if listCollectionID != "" {
		activeFilters = append(activeFilters, fmt.Sprintf("collection-id=%s", listCollectionID))
	}
	if listItemID != "" {
		activeFilters = append(activeFilters, fmt.Sprintf("item-id=%s", listItemID))
	}
	if listCollectionsOnly {
		activeFilters = append(activeFilters, "collections-only")
	}
	if listItemsOnly {
		activeFilters = append(activeFilters, "items-only")
	}
	if len(activeFilters) > 0 {
		fmt.Printf("Filters: %s\n", strings.Join(activeFilters, ", "))
	}
	fmt.Println()

	// Display entries
	for i, entry := range results.Entries {
		displayEntry(entry, listVerbose, i+1)
	}

	// Display summary statistics
	if listVerbose {
		fmt.Printf("\nSummary:\n")
		fmt.Printf("  Total size: %s\n", formatBytes(results.TotalSize))
		fmt.Printf("  Total records: %d\n", results.TotalRecords)
		fmt.Printf("  Schemas: %s\n", strings.Join(results.UniqueSchemas, ", "))
		// Format origins for display
		formattedOrigins := make([]string, len(results.UniqueOrigins))
		for i, origin := range results.UniqueOrigins {
			formattedOrigins[i] = formatOriginForDisplay(origin)
		}
		fmt.Printf("  Origins: %s\n", strings.Join(formattedOrigins, ", "))
		fmt.Printf("  Formats: %s\n", strings.Join(results.UniqueFormats, ", "))
	}

	return nil
}

// determineCollectionType determines the collection type filter based on flags
func determineCollectionType() string {
	if listCollectionsOnly {
		return "collection"
	}
	if listItemsOnly {
		return "item"
	}
	return "" // No filter
}

func init() {
	rootCmd.AddCommand(listCmd)

	// Add flags
	listCmd.Flags().StringVar(&listSchema, "schema", "", "Filter by CUE schema (e.g., aws.#EC2Instance)")
	listCmd.Flags().StringVar(&listOrigin, "origin", "", "Filter by data origin (e.g., aws-ec2)")
	listCmd.Flags().StringVar(&listFormat, "format", "", "Filter by file format (json, yaml, csv, ndjson)")
	listCmd.Flags().BoolVarP(&listVerbose, "verbose", "v", false, "Show detailed information")
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "Limit number of results")
	listCmd.Flags().StringVar(&listSortBy, "sort-by", "timestamp", "Sort by field (timestamp, size, records, schema, origin)")
	listCmd.Flags().BoolVar(&listReverse, "reverse", false, "Reverse sort order")

	// Collection-specific flags
	listCmd.Flags().StringVar(&listCollectionID, "collection-id", "", "Filter by collection ID")
	listCmd.Flags().StringVar(&listItemID, "item-id", "", "Filter by item ID")
	listCmd.Flags().BoolVar(&listCollectionsOnly, "collections-only", false, "Show only collections")
	listCmd.Flags().BoolVar(&listItemsOnly, "items-only", false, "Show only individual items")

	// UI flags
	listCmd.Flags().BoolVar(&listFancy, "fancy", false, "Use interactive bubbletea interface with filtering")

	// Make collections-only and items-only mutually exclusive
	listCmd.MarkFlagsMutuallyExclusive("collections-only", "items-only")
}

// displayEntry displays a single catalog entry
func displayEntry(entry lister.ListEntry, verbose bool, index int) {
	// Basic info line with collection indicator
	collectionIndicator := ""
	if entry.CollectionType != nil {
		switch *entry.CollectionType {
		case "collection":
			collectionIndicator = " 📦"
		case "item":
			collectionIndicator = " 📄"
		}
	}

	// Display proquint as the primary ID (no timestamp)
	fmt.Printf("%d. %s [%s]%s\n",
		index,
		entry.Proquint,
		entry.Schema,
		collectionIndicator)

	// Format origin for display - convert hash-based origins to proquint
	displayOrigin := formatOriginForDisplay(entry.Origin)

	// Additional details
	detailsLine := fmt.Sprintf("   Origin: %s | Format: %s | Records: %d | Size: %s",
		displayOrigin,
		entry.Format,
		entry.RecordCount,
		formatBytes(entry.SizeBytes))

	// Add collection info if this is an item
	if entry.CollectionType != nil && *entry.CollectionType == "item" && entry.CollectionID != nil {
		// Convert collection ID hash to proquint for display
		collectionProquint := idgen.HashToProquint(*entry.CollectionID)
		detailsLine += fmt.Sprintf(" | Collection: %s", collectionProquint)
		if entry.ItemIndex != nil {
			detailsLine += fmt.Sprintf(" [#%d]", *entry.ItemIndex)
		}
	}

	fmt.Println(detailsLine)

	// Verbose details
	if verbose {
		fmt.Printf("   Hash: %s\n", entry.ID)
		fmt.Printf("   Data: %s\n", entry.StoredPath)
		fmt.Printf("   Metadata: %s\n", entry.MetadataPath)
		fmt.Printf("   Timestamp: %s\n", entry.ImportTimestamp)
		if entry.Confidence < 0.8 {
			fmt.Printf("   ⚠️  Low schema confidence (%.2f)\n", entry.Confidence)
		}

		// Show collection details
		if entry.CollectionType != nil {
			fmt.Printf("   Type: %s", *entry.CollectionType)
			if *entry.CollectionType == "item" && entry.ItemID != nil {
				fmt.Printf(" (Item ID: %s)", *entry.ItemID)
			}
			fmt.Println()
		}
	}

	fmt.Println()
}

// formatOriginForDisplay converts hash-based origins to human-readable format
// e.g., "3bd89e80cb116834..._item_0" becomes "govim-nupab_item_0"
func formatOriginForDisplay(origin string) string {
	// Check if origin contains "_item_" pattern (collection item origin)
	if idx := strings.Index(origin, "_item_"); idx != -1 {
		hashPart := origin[:idx]
		itemPart := origin[idx:]
		// If the hash part looks like a hex hash (64 chars), convert to proquint
		if len(hashPart) == 64 && isHexString(hashPart) {
			return idgen.HashToProquint(hashPart) + itemPart
		}
	}
	return origin
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// formatBytes formats byte count as human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

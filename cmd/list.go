package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/lister"
	"github.com/chazu/pudl/internal/ui"
)

var (
	listSchema          string
	listOrigin          string
	listFormat          string
	listVerbose         bool
	listLimit           int
	listSortBy          string
	listReverse         bool
	listCollectionID    string
	listItemID          string
	listCollectionsOnly bool
	listItemsOnly       bool
	listFancy           bool
	listPage            int
	listPerPage         int
	listArtifacts       bool
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List imported data in the PUDL data lake",
	Long: `List and query imported data in the PUDL data lake with filtering and sorting options.

This command displays imported data from the active catalog, including metadata
such as schema assignments, timestamps, sizes, and record counts. In a repository
workspace all records in its local catalog are visible unless --origin is supplied.

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
- --limit: Cap the total number of results across all pages (default: no cap)
- --sort-by: Sort by field (timestamp, size, records, schema, origin)
- --reverse: Reverse sort order
- --fancy: Use interactive bubbletea interface with filtering (press / to filter, enter to show details with raw data)

Examples:
    pudl list                                    # List the active catalog
    pudl list --schema aws.#EC2Instance          # List only EC2 instances
    pudl list --origin k8s-pods                  # List Kubernetes pod data
    pudl list --format ndjson --verbose         # List NDJSON collections with details
    pudl list --collections-only                # Show only collections
    pudl list --items-only                      # Show only individual items
    pudl list --collection-id my-collection     # Show items from specific collection
    pudl list --sort-by size --reverse          # List by size, largest first
    pudl list --limit 10                        # Show only first 10 entries
    pudl list --fancy                           # Interactive list with filtering and detailed view`,
	RunE: pudlRunE(runListCommand),
}

// runListCommand contains the actual list logic with structured error handling
func runListCommand(cmd *cobra.Command, args []string) error {
	// Load configuration to get data directory
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return err // Already a PUDLError from loadEffectiveConfig()
	}

	// Create lister
	l, err := lister.New(cfg.DataPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize lister", err)
	}
	defer l.Close()

	// Determine entry type filter.
	// Live entry_type taxonomy: "observe" (ingested/observed data),
	// "manifest" + "manifest-action" (run outputs). Default shows everything;
	// --artifacts narrows to run outputs.
	var entryTypes []string // default: no filter (show everything)
	if listArtifacts {
		entryTypes = []string{"manifest", "manifest-action"}
	}

	// Set up filter options
	filters := lister.FilterOptions{
		Schema:         listSchema,
		Origin:         listOrigin,
		Format:         listFormat,
		CollectionID:   listCollectionID,
		CollectionType: determineCollectionType(),
		ItemID:         listItemID,
		EntryTypes:     entryTypes,
	}

	// Set up display options
	displayOpts := lister.DisplayOptions{
		Verbose: listVerbose,
		Limit:   listLimit,
		SortBy:  listSortBy,
		Reverse: listReverse,
		Page:    listPage,
		PerPage: listPerPage,
	}

	// List data
	results, err := l.ListData(filters, displayOpts)
	if err != nil {
		return err // Already a PUDLError from lister
	}

	// Display results
	if len(results.Entries) == 0 {
		// Handle JSON output for empty results
		output := GetOutputWriter()
		if output.Format == ui.OutputFormatJSON {
			return output.WriteJSON(ui.ListOutput{
				Entries:      []ui.EntryOutput{},
				TotalEntries: results.TotalEntries,
				TotalMatched: results.TotalMatched,
				TotalPages:   results.TotalPages,
				CurrentPage:  results.CurrentPage,
			})
		}
		fmt.Fprintln(outw(), "No data found matching the specified criteria.")
		printAnchoredSchemaHint(l, filters)
		return nil
	}

	// Handle JSON output
	output := GetOutputWriter()
	if output.Format == ui.OutputFormatJSON {
		return outputListAsJSON(output, results)
	}

	// Use fancy bubbletea UI if requested
	if listFancy {
		return ui.RunInteractiveList(results.Entries, listVerbose)
	}

	// Traditional text output
	// Display summary with pagination info
	fmt.Fprintf(outw(), "Found %d entries", len(results.Entries))
	if results.TotalEntries > len(results.Entries) {
		startIdx := (results.CurrentPage-1)*listPerPage + 1
		endIdx := startIdx + len(results.Entries) - 1
		fmt.Fprintf(outw(), " (showing %d-%d of %d total, page %d of %d)", startIdx, endIdx, results.TotalEntries, results.CurrentPage, results.TotalPages)
	}
	if results.TotalMatched > results.TotalEntries {
		fmt.Fprintf(outw(), " [--limit %d of %d matching]", results.TotalEntries, results.TotalMatched)
	}
	fmt.Fprintln(outw())

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
		fmt.Fprintf(outw(), "Filters: %s\n", strings.Join(activeFilters, ", "))
	}
	fmt.Fprintln(outw())

	// Display entries
	for i, entry := range results.Entries {
		displayEntry(entry, listVerbose, i+1)
	}

	// Display summary statistics
	if listVerbose {
		fmt.Fprintf(outw(), "\nSummary:\n")
		fmt.Fprintf(outw(), "  Total size: %s\n", formatBytes(results.TotalSize))
		fmt.Fprintf(outw(), "  Total records: %d\n", results.TotalRecords)
		fmt.Fprintf(outw(), "  Schemas: %s\n", strings.Join(results.UniqueSchemas, ", "))
		// Format origins for display
		formattedOrigins := make([]string, len(results.UniqueOrigins))
		for i, origin := range results.UniqueOrigins {
			formattedOrigins[i] = formatOriginForDisplay(origin)
		}
		fmt.Fprintf(outw(), "  Origins: %s\n", strings.Join(formattedOrigins, ", "))
		fmt.Fprintf(outw(), "  Formats: %s\n", strings.Join(results.UniqueFormats, ", "))
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
	listCmd.Flags().StringVar(&listSchema, "schema", "", "Filter by CUE schema: a value with # matches whole definition names (aws.#EC2Instance, #Route); otherwise a substring")
	listCmd.Flags().StringVar(&listOrigin, "origin", "", "Filter by data origin (e.g., aws-ec2)")
	listCmd.Flags().StringVar(&listFormat, "format", "", "Filter by file format (json, yaml, csv, ndjson)")
	listCmd.Flags().BoolVarP(&listVerbose, "verbose", "v", false, "Show detailed information")
	listCmd.Flags().IntVar(&listLimit, "limit", 0, "Cap the total number of results across all pages (0 = no cap)")
	listCmd.Flags().StringVar(&listSortBy, "sort-by", "timestamp", "Sort by field (timestamp, size, records, schema, origin)")
	listCmd.Flags().BoolVar(&listReverse, "reverse", false, "Reverse sort order")
	listCmd.Flags().IntVar(&listPage, "page", 1, "Page number (1-based)")
	listCmd.Flags().IntVar(&listPerPage, "per-page", 20, "Results per page")

	// Collection-specific flags
	listCmd.Flags().StringVar(&listCollectionID, "collection-id", "", "Filter by collection ID")
	listCmd.Flags().StringVar(&listItemID, "item-id", "", "Filter by item ID")
	listCmd.Flags().BoolVar(&listCollectionsOnly, "collections-only", false, "Show only collections")
	listCmd.Flags().BoolVar(&listItemsOnly, "items-only", false, "Show only individual items")

	// Entry type flags
	listCmd.Flags().BoolVar(&listArtifacts, "artifacts", false, "Show only run outputs (manifest, manifest-action)")

	// UI flags
	listCmd.Flags().BoolVar(&listFancy, "fancy", false, "Use interactive bubbletea interface with filtering")

	// Make collections-only and items-only mutually exclusive
	listCmd.MarkFlagsMutuallyExclusive("collections-only", "items-only")

	// Register completion functions
	listCmd.RegisterFlagCompletionFunc("schema", completeSchemaNames)
	listCmd.RegisterFlagCompletionFunc("origin", completeOrigins)
	listCmd.RegisterFlagCompletionFunc("format", completeFormats)
	listCmd.RegisterFlagCompletionFunc("sort-by", completeSortByOptions)
	listCmd.RegisterFlagCompletionFunc("collection-id", completeProquintIDs)
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

	// Display proquint as the primary ID with version info
	versionStr := ""
	if entry.Version != nil && *entry.Version > 0 {
		versionStr = fmt.Sprintf(" v%d", *entry.Version)
	}
	fmt.Fprintf(outw(), "%d. %s%s [%s]%s\n",
		index,
		entry.Proquint,
		versionStr,
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

	fmt.Fprintln(outw(), detailsLine)

	// Verbose details
	if verbose {
		fmt.Fprintf(outw(), "   Hash: %s\n", entry.ID)
		fmt.Fprintf(outw(), "   Data: %s\n", entry.StoredPath)
		fmt.Fprintf(outw(), "   Metadata: %s\n", entry.MetadataPath)
		fmt.Fprintf(outw(), "   Timestamp: %s\n", entry.ImportTimestamp)
		if entry.Confidence < 0.8 {
			fmt.Fprintf(outw(), "   ⚠️  Low schema confidence (%.2f)\n", entry.Confidence)
		}

		// Show identity tracking details
		if entry.ResourceID != nil {
			fmt.Fprintf(outw(), "   Resource ID: %s\n", *entry.ResourceID)
		}
		if entry.ContentHash != nil {
			fmt.Fprintf(outw(), "   Content Hash: %s\n", *entry.ContentHash)
		}
		if entry.Version != nil {
			fmt.Fprintf(outw(), "   Version: %d\n", *entry.Version)
		}

		// Show collection details
		if entry.CollectionType != nil {
			fmt.Fprintf(outw(), "   Type: %s", *entry.CollectionType)
			if *entry.CollectionType == "item" && entry.ItemID != nil {
				fmt.Fprintf(outw(), " (Item ID: %s)", *entry.ItemID)
			}
			fmt.Fprintln(outw())
		}
	}

	fmt.Fprintln(outw())
}

// formatOriginForDisplay converts hash-based origins to human-readable format
// For collection items, strips the "_item_X" suffix since that info is shown separately
// e.g., "3bd89e80cb116834..._item_0" becomes "govim-nupab"
func formatOriginForDisplay(origin string) string {
	// Check if origin contains "_item_" pattern (collection item origin)
	if idx := strings.Index(origin, "_item_"); idx != -1 {
		hashPart := origin[:idx]
		// If the hash part looks like a hex hash (64 chars), convert to proquint
		// and drop the _item_X suffix (it's shown in the Collection: field)
		if len(hashPart) == 64 && isHexString(hashPart) {
			return idgen.HashToProquint(hashPart)
		}
	}
	return origin
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
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

// outputListAsJSON outputs list results as JSON
func outputListAsJSON(output *ui.OutputWriter, results *lister.ListResults) error {
	entries := make([]ui.EntryOutput, len(results.Entries))
	for i, e := range results.Entries {
		entries[i] = ui.EntryOutput{
			ID:              e.ID,
			Proquint:        e.Proquint,
			Schema:          e.Schema,
			Origin:          e.Origin,
			Format:          e.Format,
			SizeBytes:       e.SizeBytes,
			RecordCount:     e.RecordCount,
			ImportTimestamp: e.ImportTimestamp,
			StoredPath:      e.StoredPath,
			MetadataPath:    e.MetadataPath,
			Confidence:      e.Confidence,
			CollectionType:  e.CollectionType,
			CollectionID:    e.CollectionID,
			ItemID:          e.ItemID,
			ItemIndex:       e.ItemIndex,
		}
	}

	listOutput := ui.ListOutput{
		Entries:      entries,
		TotalEntries: results.TotalEntries,
		TotalMatched: results.TotalMatched,
		TotalPages:   results.TotalPages,
		CurrentPage:  results.CurrentPage,
		Summary: &ui.ListSummary{
			TotalSize:     results.TotalSize,
			TotalRecords:  results.TotalRecords,
			UniqueSchemas: results.UniqueSchemas,
			UniqueOrigins: results.UniqueOrigins,
			UniqueFormats: results.UniqueFormats,
		},
	}

	return output.WriteJSON(listOutput)
}

// printAnchoredSchemaHint explains an empty result from a --schema value that
// names a definition: such values match whole definition names, so a partial
// name (e.g. "#Rout") finds nothing although a fragment search would.
func printAnchoredSchemaHint(l *lister.Lister, filters lister.FilterOptions) {
	i := strings.LastIndex(filters.Schema, "#")
	if i < 0 || i == len(filters.Schema)-1 {
		return
	}
	fragment := filters.Schema[i+1:]
	filters.Schema = fragment
	results, err := l.ListData(filters, lister.DisplayOptions{Page: 1, PerPage: 1})
	if err != nil || results.TotalMatched == 0 {
		return
	}
	fmt.Fprintf(outw(), "Hint: --schema values containing '#' match whole definition names; %d entries match the fragment %q (try --schema %s).\n", results.TotalMatched, fragment, fragment)
}

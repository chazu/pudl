package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/lister"
)

var (
	listSchema     string
	listOrigin     string
	listFormat     string
	listVerbose    bool
	listLimit      int
	listSortBy     string
	listReverse    bool
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
- --format: Filter by file format (json, yaml, csv)

Display Options:
- --verbose: Show detailed information including file paths
- --limit: Limit number of results (default: 50)
- --sort-by: Sort by field (timestamp, size, records, schema, origin)
- --reverse: Reverse sort order

Examples:
    pudl list                                    # List all imported data
    pudl list --schema aws.#EC2Instance          # List only EC2 instances
    pudl list --origin k8s-pods                  # List Kubernetes pod data
    pudl list --format yaml --verbose           # List YAML files with details
    pudl list --sort-by size --reverse          # List by size, largest first
    pudl list --limit 10                        # Show only first 10 entries`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration to get data directory
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		// Create lister
		l := lister.New(cfg.DataPath)

		// Set up filter options
		filters := lister.FilterOptions{
			Schema: listSchema,
			Origin: listOrigin,
			Format: listFormat,
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
			log.Fatalf("Failed to list data: %v", err)
		}

		// Display results
		if len(results.Entries) == 0 {
			fmt.Println("No data found matching the specified criteria.")
			return
		}

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
			fmt.Printf("  Origins: %s\n", strings.Join(results.UniqueOrigins, ", "))
			fmt.Printf("  Formats: %s\n", strings.Join(results.UniqueFormats, ", "))
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)

	// Add flags
	listCmd.Flags().StringVar(&listSchema, "schema", "", "Filter by CUE schema (e.g., aws.#EC2Instance)")
	listCmd.Flags().StringVar(&listOrigin, "origin", "", "Filter by data origin (e.g., aws-ec2)")
	listCmd.Flags().StringVar(&listFormat, "format", "", "Filter by file format (json, yaml, csv)")
	listCmd.Flags().BoolVarP(&listVerbose, "verbose", "v", false, "Show detailed information")
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "Limit number of results")
	listCmd.Flags().StringVar(&listSortBy, "sort-by", "timestamp", "Sort by field (timestamp, size, records, schema, origin)")
	listCmd.Flags().BoolVar(&listReverse, "reverse", false, "Reverse sort order")
}

// displayEntry displays a single catalog entry
func displayEntry(entry lister.ListEntry, verbose bool, index int) {
	// Basic info line
	fmt.Printf("%d. %s [%s] (%s)\n", 
		index, 
		entry.ID, 
		entry.Schema, 
		entry.ImportTimestamp)

	// Additional details
	fmt.Printf("   Origin: %s | Format: %s | Records: %d | Size: %s\n",
		entry.Origin,
		entry.Format,
		entry.RecordCount,
		formatBytes(entry.SizeBytes))

	// Verbose details
	if verbose {
		fmt.Printf("   Data: %s\n", entry.StoredPath)
		fmt.Printf("   Metadata: %s\n", entry.MetadataPath)
		if entry.Confidence < 0.8 {
			fmt.Printf("   ⚠️  Low schema confidence (%.2f)\n", entry.Confidence)
		}
	}

	fmt.Println()
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

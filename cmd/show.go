package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"pudl/internal/config"
	"pudl/internal/lister"
)

var (
	showMetadata bool
	showRaw      bool
)

// showCmd represents the show command
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show detailed information about a specific data entry",
	Long: `Show detailed information about a specific data entry including its content,
metadata, and schema information.

The ID parameter should be the unique identifier of the data entry as shown
in the 'pudl list' command output.

Display Options:
- --metadata: Show the metadata file content
- --raw: Show the raw imported data content

Examples:
    pudl show 20250825_222510_test-data           # Show basic info
    pudl show 20250825_222510_test-data --metadata # Show with metadata
    pudl show 20250825_222510_test-data --raw      # Show with raw data
    pudl show 20250825_222510_test-data --metadata --raw # Show everything`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		entryID := args[0]

		// Load configuration to get data directory
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		// Create lister to find the entry
		l := lister.New(cfg.DataPath)

		// Find the specific entry
		entry, err := l.FindEntry(entryID)
		if err != nil {
			log.Fatalf("Failed to find entry: %v", err)
		}

		if entry == nil {
			fmt.Printf("Entry with ID '%s' not found.\n", entryID)
			fmt.Println("\nUse 'pudl list' to see available entries.")
			return
		}

		// Display entry information
		displayDetailedEntry(*entry, showMetadata, showRaw)
	},
}

func init() {
	rootCmd.AddCommand(showCmd)

	// Add flags
	showCmd.Flags().BoolVar(&showMetadata, "metadata", false, "Show metadata file content")
	showCmd.Flags().BoolVar(&showRaw, "raw", false, "Show raw data content")
}

// displayDetailedEntry displays detailed information about a single entry
func displayDetailedEntry(entry lister.ListEntry, includeMetadata, includeRaw bool) {
	fmt.Printf("Entry: %s\n", entry.ID)
	fmt.Printf("Schema: %s\n", entry.Schema)
	fmt.Printf("Origin: %s\n", entry.Origin)
	fmt.Printf("Format: %s\n", entry.Format)
	fmt.Printf("Import Time: %s\n", entry.ImportTimestamp)
	fmt.Printf("Records: %d\n", entry.RecordCount)
	fmt.Printf("Size: %s\n", formatBytes(entry.SizeBytes))
	fmt.Printf("Confidence: %.2f\n", entry.Confidence)
	fmt.Printf("Data Path: %s\n", entry.StoredPath)
	fmt.Printf("Metadata Path: %s\n", entry.MetadataPath)

	if entry.Confidence < 0.8 {
		fmt.Printf("⚠️  Low schema confidence - data may not match assigned schema\n")
	}

	// Show metadata if requested
	if includeMetadata {
		fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
		fmt.Printf("METADATA\n")
		fmt.Printf(strings.Repeat("=", 60) + "\n")

		metadataContent, err := os.ReadFile(entry.MetadataPath)
		if err != nil {
			fmt.Printf("Error reading metadata: %v\n", err)
		} else {
			// Pretty print JSON metadata
			var metadata map[string]interface{}
			if err := json.Unmarshal(metadataContent, &metadata); err != nil {
				fmt.Printf("Error parsing metadata: %v\n", err)
				fmt.Printf("%s\n", string(metadataContent))
			} else {
				prettyMetadata, err := json.MarshalIndent(metadata, "", "  ")
				if err != nil {
					fmt.Printf("%s\n", string(metadataContent))
				} else {
					fmt.Printf("%s\n", string(prettyMetadata))
				}
			}
		}
	}

	// Show raw data if requested
	if includeRaw {
		fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
		fmt.Printf("RAW DATA\n")
		fmt.Printf(strings.Repeat("=", 60) + "\n")

		rawContent, err := os.ReadFile(entry.StoredPath)
		if err != nil {
			fmt.Printf("Error reading raw data: %v\n", err)
		} else {
			// Try to pretty print based on format
			switch strings.ToLower(entry.Format) {
			case "json":
				var data interface{}
				if err := json.Unmarshal(rawContent, &data); err != nil {
					fmt.Printf("%s\n", string(rawContent))
				} else {
					prettyData, err := json.MarshalIndent(data, "", "  ")
					if err != nil {
						fmt.Printf("%s\n", string(rawContent))
					} else {
						fmt.Printf("%s\n", string(prettyData))
					}
				}
			case "yaml":
				var data interface{}
				if err := yaml.Unmarshal(rawContent, &data); err != nil {
					fmt.Printf("%s\n", string(rawContent))
				} else {
					prettyData, err := yaml.Marshal(data)
					if err != nil {
						fmt.Printf("%s\n", string(rawContent))
					} else {
						fmt.Printf("%s\n", string(prettyData))
					}
				}
			default:
				fmt.Printf("%s\n", string(rawContent))
			}
		}
	}

	fmt.Println()
}

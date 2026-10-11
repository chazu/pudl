package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/lister"
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
- --json: Print {"entry": ..., "metadata": ..., "raw": ...} as one JSON document;
  stored JSON is embedded exactly, other formats appear as "raw_text"

Examples:
    pudl show 20250825_222510_test-data           # Show basic info
    pudl show 20250825_222510_test-data --metadata # Show with metadata
    pudl show 20250825_222510_test-data --raw      # Show with raw data
    pudl show 20250825_222510_test-data --metadata --raw # Show everything`,
	Args: cobra.ExactArgs(1),
	RunE: pudlRunE(runShowCommand),
}

// runShowCommand contains the actual show logic with structured error handling
func runShowCommand(cmd *cobra.Command, args []string) error {
	entryID := args[0]

	// Load configuration to get data directory
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return err // Already a PUDLError from loadEffectiveConfig()
	}

	// Create lister to find the entry
	l, err := lister.New(cfg.DataPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize lister", err)
	}
	defer l.Close()

	// Find the specific entry
	entry, err := l.FindEntry(entryID)
	if err != nil {
		return err // Already a PUDLError from lister.FindEntry()
	}

	if entry == nil {
		return errors.NewInputError(
			fmt.Sprintf("Entry with ID '%s' not found", entryID),
			"Use 'pudl list' to see available entries",
			"Check that the entry ID is correct")
	}

	if showField != "" {
		return showEntryField(entry, showField)
	}
	if showHistory {
		return showEntryHistory(cmd.Context(), entry)
	}
	if jsonOutput {
		return writeEntryJSON(*entry, showMetadata, showRaw)
	}

	// Display entry information
	displayDetailedEntry(*entry, showMetadata, showRaw)
	return nil
}

func init() {
	rootCmd.AddCommand(showCmd)

	// Add flags
	showCmd.Flags().BoolVar(&showMetadata, "metadata", false, "Show metadata file content")
	showCmd.Flags().BoolVar(&showRaw, "raw", false, "Show raw data content")

	showCmd.Flags().StringVar(&showField, "field", "", "Print one payload field as JSON (nested paths and wildcards supported)")
	showCmd.Flags().BoolVar(&showHistory, "history", false, "Show this resource's stored versions and snapshot observations")
	showCmd.MarkFlagsMutuallyExclusive("field", "history")
	showCmd.MarkFlagsMutuallyExclusive("field", "raw")
	showCmd.MarkFlagsMutuallyExclusive("field", "metadata")
	showCmd.MarkFlagsMutuallyExclusive("history", "raw")
	showCmd.MarkFlagsMutuallyExclusive("history", "metadata")

	// Register completion for positional argument (proquint ID)
	showCmd.ValidArgsFunction = completeEntryIDs
}

// displayDetailedEntry displays detailed information about a single entry
func displayDetailedEntry(entry lister.ListEntry, includeMetadata, includeRaw bool) {
	fmt.Fprintf(outw(), "Entry: %s\n", entry.Proquint)
	fmt.Fprintf(outw(), "Hash: %s\n", entry.ID)
	fmt.Fprintf(outw(), "Schema: %s\n", entry.Schema)
	fmt.Fprintf(outw(), "Origin: %s\n", entry.Origin)
	fmt.Fprintf(outw(), "Format: %s\n", entry.Format)
	fmt.Fprintf(outw(), "Import Time: %s\n", entry.ImportTimestamp)
	fmt.Fprintf(outw(), "Records: %d\n", entry.RecordCount)
	fmt.Fprintf(outw(), "Size: %s\n", formatBytes(entry.SizeBytes))
	fmt.Fprintf(outw(), "Confidence: %.2f\n", entry.Confidence)
	fmt.Fprintf(outw(), "Data Path: %s\n", entry.StoredPath)
	fmt.Fprintf(outw(), "Metadata Path: %s\n", entry.MetadataPath)

	if entry.Confidence < 0.8 {
		fmt.Fprintf(outw(), "⚠️  Low schema confidence - data may not match assigned schema\n")
	}

	// Show metadata if requested
	if includeMetadata {
		separator := strings.Repeat("=", 60)
		fmt.Fprintf(outw(), "\n%s\n", separator)
		fmt.Fprintf(outw(), "METADATA\n")
		fmt.Fprintf(outw(), "%s\n", separator)

		metadataContent, err := os.ReadFile(entry.MetadataPath)
		if err != nil {
			fmt.Fprintf(outw(), "Error reading metadata: %v\n", err)
		} else {
			// Pretty print JSON metadata
			var metadata map[string]interface{}
			if err := json.Unmarshal(metadataContent, &metadata); err != nil {
				fmt.Fprintf(outw(), "Error parsing metadata: %v\n", err)
				fmt.Fprintf(outw(), "%s\n", string(metadataContent))
			} else {
				prettyMetadata, err := json.MarshalIndent(metadata, "", "  ")
				if err != nil {
					fmt.Fprintf(outw(), "%s\n", string(metadataContent))
				} else {
					fmt.Fprintf(outw(), "%s\n", string(prettyMetadata))
				}
			}
		}
	}

	// Show raw data if requested
	if includeRaw {
		separator := strings.Repeat("=", 60)
		fmt.Fprintf(outw(), "\n%s\n", separator)
		fmt.Fprintf(outw(), "RAW DATA\n")
		fmt.Fprintf(outw(), "%s\n", separator)

		rawContent, err := os.ReadFile(entry.StoredPath)
		if err != nil {
			fmt.Fprintf(outw(), "Error reading raw data: %v\n", err)
		} else {
			// Try to pretty print based on format
			switch strings.ToLower(entry.Format) {
			case "json":
				var data interface{}
				if err := json.Unmarshal(rawContent, &data); err != nil {
					fmt.Fprintf(outw(), "%s\n", string(rawContent))
				} else {
					prettyData, err := json.MarshalIndent(data, "", "  ")
					if err != nil {
						fmt.Fprintf(outw(), "%s\n", string(rawContent))
					} else {
						fmt.Fprintf(outw(), "%s\n", string(prettyData))
					}
				}
			case "yaml":
				var data interface{}
				if err := yaml.Unmarshal(rawContent, &data); err != nil {
					fmt.Fprintf(outw(), "%s\n", string(rawContent))
				} else {
					prettyData, err := yaml.Marshal(data)
					if err != nil {
						fmt.Fprintf(outw(), "%s\n", string(rawContent))
					} else {
						fmt.Fprintf(outw(), "%s\n", string(prettyData))
					}
				}
			default:
				fmt.Fprintf(outw(), "%s\n", string(rawContent))
			}
		}
	}

}

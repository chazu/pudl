package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/lister"
)

var (
	showMetadata bool
	showRaw      bool
	showMetaOnly bool
)

// showCmd represents the show command
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a data entry: its type, compliance and stored record",
	Long: `Show a data entry: its catalog details (type, origin, format, paths), its
compliance and the stored record itself.

A compliance section lists each policy applicable to the entry's type with the
resource's stored status (provisional when the type is a draft), its highest
violation severity, and its violations (rule, severity, path, kind, declaring
policy). --json includes it as "compliance".

The record follows under RAW DATA: stored JSON indented with its values
unchanged, YAML re-rendered, other formats verbatim. Sensitive fields were
redacted when the record was stored; show prints what is stored.

The ID parameter is the entry's proquint or hash as shown by 'pudl list'.

Display Options:
- --meta-only: Show the catalog details and compliance without the record
- --metadata: Also show the metadata file content
- --raw: Show the record (the default; kept for compatibility)
- --field PATH: Print one payload field as JSON
- --history: Show the resource's stored versions and snapshot observations
- --json: Print {"entry": ..., "raw": ..., "compliance": ...} as one JSON
  document ("metadata" with --metadata; no "raw" with --meta-only); stored JSON
  is embedded exactly, other formats appear as "raw_text"

Examples:
    pudl show vomap-foqab                  # Details, compliance and the record
    pudl show vomap-foqab --meta-only      # Details and compliance only
    pudl show vomap-foqab --metadata       # Also the metadata file
    pudl show vomap-foqab --field name     # One field as JSON
    pudl show vomap-foqab --json           # Everything as one JSON document`,
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
	// The record is shown unless --meta-only asks for the details alone;
	// --raw is the default and stays accepted.
	includeData := !showMetaOnly
	if jsonOutput {
		return writeEntryJSON(*entry, showMetadata, includeData)
	}

	// Display entry information
	displayDetailedEntry(*entry, showMetadata, includeData)
	return nil
}

func init() {
	rootCmd.AddCommand(showCmd)

	// Add flags
	showCmd.Flags().BoolVar(&showMetadata, "metadata", false, "Show metadata file content")
	showCmd.Flags().BoolVar(&showRaw, "raw", false, "Show the stored record (the default; kept for compatibility)")
	showCmd.Flags().BoolVar(&showMetaOnly, "meta-only", false, "Show catalog details and compliance without the stored record")

	showCmd.Flags().StringVar(&showField, "field", "", "Print one payload field as JSON (nested paths and wildcards supported)")
	showCmd.Flags().BoolVar(&showHistory, "history", false, "Show this resource's stored versions and snapshot observations")
	showCmd.MarkFlagsMutuallyExclusive("field", "history")
	showCmd.MarkFlagsMutuallyExclusive("field", "raw")
	showCmd.MarkFlagsMutuallyExclusive("field", "metadata")
	showCmd.MarkFlagsMutuallyExclusive("history", "raw")
	showCmd.MarkFlagsMutuallyExclusive("history", "metadata")
	for _, flag := range []string{"raw", "field", "history"} {
		showCmd.MarkFlagsMutuallyExclusive("meta-only", flag)
	}

	// Register completion for positional argument (proquint ID)
	showCmd.ValidArgsFunction = completeEntryIDs
}

// displayDetailedEntry displays detailed information about a single entry
func displayDetailedEntry(entry lister.ListEntry, includeMetadata, includeData bool) {
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
	printCompliance(entryCompliance(entry))

	// Show metadata if requested
	if includeMetadata {
		printShowSection("METADATA")

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

	// Show the stored record unless --meta-only
	if includeData {
		printEntryData(entry)
	}
}

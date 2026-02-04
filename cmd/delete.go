package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/lister"
	"pudl/internal/ui"
)

var (
	deleteForce   bool
	deleteCascade bool
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a data entry from the catalog",
	Long: `Delete a data entry from the PUDL catalog, including its data file and metadata.

The ID parameter should be the proquint identifier of the data entry as shown
in the 'pudl list' command output.

For collections, use --cascade to also delete all items in the collection.
Without --cascade, deleting a collection with items will fail.

Examples:
    pudl delete mivof-duhij                    # Delete with confirmation prompt
    pudl delete mivof-duhij --force            # Delete without confirmation
    pudl delete govim-nupab --cascade          # Delete collection and all its items
    pudl delete mivof-duhij --json             # Output result as JSON`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)
		if err := runDeleteCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Delete without confirmation prompt")
	deleteCmd.Flags().BoolVar(&deleteCascade, "cascade", false, "Delete collection and all its items")

	deleteCmd.ValidArgsFunction = completeEntryIDs
}

func runDeleteCommand(cmd *cobra.Command, args []string) error {
	entryID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	l, err := lister.New(cfg.DataPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize lister", err)
	}
	defer l.Close()

	// Find the entry
	entry, err := l.FindEntry(entryID)
	if err != nil {
		return err
	}
	if entry == nil {
		return errors.NewInputError(
			fmt.Sprintf("Entry with ID '%s' not found", entryID),
			"Use 'pudl list' to see available entries",
			"Check that the entry ID is correct")
	}

	// Check if this is a collection with items
	var itemsToDelete []lister.ListEntry
	isCollection := entry.CollectionType != nil && *entry.CollectionType == "collection"

	if isCollection {
		items, err := l.GetCollectionItems(entry.ID)
		if err != nil {
			return errors.NewSystemError("Failed to get collection items", err)
		}
		if len(items) > 0 {
			if !deleteCascade {
				return errors.NewInputError(
					fmt.Sprintf("Collection '%s' has %d items", entry.Proquint, len(items)),
					"Use --cascade to delete the collection and all its items",
					"Or delete items individually first")
			}
			itemsToDelete = items
		}
	}

	// Confirmation prompt (unless --force)
	output := GetOutputWriter()
	if !deleteForce && output.Format != ui.OutputFormatJSON {
		if !confirmDelete(entry, itemsToDelete) {
			fmt.Println("Delete cancelled.")
			return nil
		}
	}

	// Perform deletion
	result, err := l.DeleteEntry(entry.ID, deleteCascade)
	if err != nil {
		return err
	}

	// Output result
	if output.Format == ui.OutputFormatJSON {
		return output.WriteJSON(result)
	}

	// Human-readable output
	fmt.Printf("✅ Deleted entry: %s\n", entry.Proquint)
	if result.ItemsDeleted > 0 {
		fmt.Printf("   Also deleted %d collection items\n", result.ItemsDeleted)
	}
	if result.DataFileDeleted {
		fmt.Printf("   Removed data file: %s\n", entry.StoredPath)
	}
	if result.MetadataFileDeleted {
		fmt.Printf("   Removed metadata file: %s\n", entry.MetadataPath)
	}

	return nil
}

func confirmDelete(entry *lister.ListEntry, items []lister.ListEntry) bool {
	fmt.Printf("About to delete:\n")
	fmt.Printf("  Entry: %s [%s]\n", entry.Proquint, entry.Schema)
	fmt.Printf("  Origin: %s\n", entry.Origin)
	fmt.Printf("  Size: %d bytes\n", entry.SizeBytes)

	if len(items) > 0 {
		fmt.Printf("  Collection items to delete: %d\n", len(items))
		for i, item := range items {
			if i >= 5 {
				fmt.Printf("    ... and %d more\n", len(items)-5)
				break
			}
			fmt.Printf("    - %s [%s]\n", item.Proquint, item.Schema)
		}
	}

	fmt.Print("\nAre you sure? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	return response == "y" || response == "yes"
}


package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/idgen"
)

var dataLatestRaw bool

var dataLatestCmd = &cobra.Command{
	Use:   "latest <definition> <method>",
	Short: "Show the most recent artifact for a definition and method",
	Long: `Display the most recent method execution artifact.

Examples:
    pudl data latest prod_instance list
    pudl data latest prod_instance list --raw`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDataLatest(args[0], args[1])
	},
}

func init() {
	dataCmd.AddCommand(dataLatestCmd)
	dataLatestCmd.Flags().BoolVar(&dataLatestRaw, "raw", false, "Print raw artifact JSON only")
}

func runDataLatest(definition, method string) error {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return fmt.Errorf("failed to open catalog: %w", err)
	}
	defer db.Close()

	entry, err := db.GetLatestArtifact(definition, method)
	if err != nil {
		return err
	}

	if dataLatestRaw {
		data, err := os.ReadFile(entry.StoredPath)
		if err != nil {
			return fmt.Errorf("failed to read artifact file: %w", err)
		}
		fmt.Print(string(data))
		return nil
	}

	proquint := idgen.HashToProquint(entry.ID)
	fmt.Printf("Artifact: %s\n", proquint)
	fmt.Printf("Definition: %s\n", ptrStr(entry.Definition))
	fmt.Printf("Method: %s\n", ptrStr(entry.Method))
	fmt.Printf("Timestamp: %s\n", entry.ImportTimestamp.Format(time.RFC3339))
	fmt.Printf("Size: %s\n", formatBytes(entry.SizeBytes))
	fmt.Printf("Path: %s\n", entry.StoredPath)
	fmt.Printf("Run ID: %s\n", ptrStr(entry.RunID))

	if entry.Tags != nil {
		fmt.Printf("Tags: %s\n", *entry.Tags)
	}

	return nil
}

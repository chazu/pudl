package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/idgen"
)

var (
	dataSearchDefinition string
	dataSearchMethod     string
	dataSearchLimit      int
)

var dataSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search stored artifacts",
	Long: `Search method execution artifacts by definition, method, or other criteria.

Examples:
    pudl data search
    pudl data search --definition prod_instance
    pudl data search --definition prod_instance --method list
    pudl data search --limit 10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDataSearch()
	},
}

func init() {
	dataCmd.AddCommand(dataSearchCmd)
	dataSearchCmd.Flags().StringVar(&dataSearchDefinition, "definition", "", "Filter by definition name")
	dataSearchCmd.Flags().StringVar(&dataSearchMethod, "method", "", "Filter by method name")
	dataSearchCmd.Flags().IntVar(&dataSearchLimit, "limit", 50, "Maximum number of results")
}

func runDataSearch() error {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return fmt.Errorf("failed to open catalog: %w", err)
	}
	defer db.Close()

	entries, err := db.SearchArtifacts(database.ArtifactFilters{
		Definition: dataSearchDefinition,
		Method:     dataSearchMethod,
		Limit:      dataSearchLimit,
	})
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No artifacts found.")
		return nil
	}

	fmt.Printf("Found %d artifact(s)\n\n", len(entries))
	for i, e := range entries {
		proquint := idgen.HashToProquint(e.ID)
		def := ptrStr(e.Definition)
		method := ptrStr(e.Method)
		fmt.Printf("%d. %s  %s.%s  %s  %s\n",
			i+1, proquint, def, method,
			formatBytes(e.SizeBytes),
			e.ImportTimestamp.Format(time.RFC3339))
		fmt.Printf("   Path: %s\n\n", e.StoredPath)
	}

	return nil
}

func ptrStr(s *string) string {
	if s == nil {
		return "<none>"
	}
	return *s
}

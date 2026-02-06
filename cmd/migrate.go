package cmd

import (
	"github.com/spf13/cobra"
)

// migrateCmd represents the top-level migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run data migrations",
	Long: `Run data migrations to upgrade the PUDL catalog.

Available migrations:
  identity    Backfill resource identity tracking for existing entries`,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}

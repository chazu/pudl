package cmd

import (
	"github.com/spf13/cobra"
)

var (
	schemaVerbose bool
	schemaPackage string
)

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:     "schema",
	Aliases: []string{"s"},
	Short:   "Manage CUE schemas for data validation",
	Long: `Manage CUE schemas for import validation and resource identity.

Schemas come from the project and its explicit vendored dependencies, or the
personal workspace selected by --global. Use list/show for discovery, add/new
for authoring, and reinfer/migrate for reviewed maintenance. Inspect physical
paths with pudl config --paths --json. Git, your editor, and cue manage the files
directly; see docs/retired-commands.md for replacements.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: show help
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}

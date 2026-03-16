package cmd

import (
	"github.com/spf13/cobra"
)

var (
	defVerbose bool
	defSchema  string
)

// definitionCmd represents the definition command
var definitionCmd = &cobra.Command{
	Use:     "definition",
	Aliases: []string{"def", "d"},
	Short:   "Manage definitions (named instances of schemas)",
	Long: `Manage definitions that bind concrete values to schemas.

A definition is a named instance of a schema with specific configuration
and socket wiring to other definitions.

Available subcommands:
- list:     Show available definitions
- show:     Display definition details
- validate: Validate definitions against their schemas
- graph:    Show definition dependency graph

Examples:
    pudl definition list
    pudl def show prod_instance
    pudl def validate
    pudl def graph`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(definitionCmd)
}

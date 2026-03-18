package cmd

import (
	"github.com/spf13/cobra"
)

// methodCmd represents the method command
var methodCmd = &cobra.Command{
	Use:     "method",
	Aliases: []string{"meth"},
	Short:   "Execute and inspect model methods",
	Long: `Execute and inspect methods defined on models.

Available subcommands:
- run:   Execute a method on a definition
- list:  List methods available for a definition

Examples:
    pudl method list prod_instance
    pudl method run prod_instance create`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(methodCmd)
}

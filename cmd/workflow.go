package cmd

import (
	"github.com/spf13/cobra"
)

var workflowCmd = &cobra.Command{
	Use:     "workflow",
	Aliases: []string{"wf"},
	Short:   "Manage and execute workflows",
	Long: `Manage and execute multi-step workflow pipelines.

Available subcommands:
- run:      Execute a workflow
- list:     List available workflows
- show:     Show workflow details
- validate: Validate a workflow
- history:  Show past run history`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(workflowCmd)
}

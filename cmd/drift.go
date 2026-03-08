package cmd

import (
	"github.com/spf13/cobra"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect drift between declared and live state",
	Long: `Compare declared definition state against live state from artifacts.

Available subcommands:
- check:  Run drift detection on a definition
- report: Show saved drift reports`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(driftCmd)
}

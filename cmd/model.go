package cmd

import (
	"github.com/spf13/cobra"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage #SystemModel definitions",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	Long: `Manage registered #SystemModel definitions.

A #SystemModel is registered as a CUE definition inheriting #SystemModel in the
schema repository (project .pudl/schema or global ~/.pudl/schema).`,
}

func init() {
	rootCmd.AddCommand(modelCmd)
	modelCmd.AddCommand(modelNewCmd)
}
